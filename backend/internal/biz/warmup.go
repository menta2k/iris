package biz

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// IP warmup gradually raises a VMTA's outbound volume per receiving-domain
// family (MBP) over a curve of daily caps, building sender reputation, then
// completes (the cap is removed entirely). KumoMTA has no native warmup, so the
// per-day cap is rendered as a max_message_rate on the egress path for the
// matching (egress_source, MBP bucket); see writeEgressPaths.

// MBP buckets — receiving-domain families that are warmed independently because
// each provider tracks reputation separately. "default" covers everything else.
const (
	MBPGmail     = "gmail"
	MBPMicrosoft = "microsoft"
	MBPYahoo     = "yahoo"
	MBPDefault   = "default"
)

// warmupBuckets is the canonical bucket order (default last).
var warmupBuckets = []string{MBPGmail, MBPMicrosoft, MBPYahoo, MBPDefault}

// ValidMBPBucket reports whether b is a known MBP bucket.
func ValidMBPBucket(b string) bool {
	switch b {
	case MBPGmail, MBPMicrosoft, MBPYahoo, MBPDefault:
		return true
	default:
		return false
	}
}

// Warmup lifecycle statuses.
const (
	WarmupScheduled = "scheduled" // start date not reached yet
	WarmupActive    = "active"    // ramping per the curve
	WarmupPaused    = "paused"    // held at the current cap (StartDate frozen by the worker)
	WarmupCompleted = "completed" // past the curve; cap removed
)

// ValidWarmupStatus reports whether s is a known warmup status.
func ValidWarmupStatus(s string) bool {
	switch s {
	case WarmupScheduled, WarmupActive, WarmupPaused, WarmupCompleted:
		return true
	default:
		return false
	}
}

// WarmupStage is one segment of a ramp curve covering ramp days [DayFrom, DayTo]
// inclusive (1-based; day 1 = StartDate), with a per-MBP messages-per-day cap.
type WarmupStage struct {
	DayFrom int            `json:"day_from"`
	DayTo   int            `json:"day_to"`
	Caps    map[string]int `json:"caps"` // bucket -> messages/day (0/absent = no cap)
}

// WarmupSchedule ramps one VMTA's volume from StartDate following Stages, then
// completes. Stages must be contiguous, 1-based, and ascending.
type WarmupSchedule struct {
	ID           string
	VMTAID       string
	VMTAName     string // resolved sending source name, read-only for rendering/display
	StartDate    time.Time
	Curve        string // template name the stages were resolved from
	Stages       []WarmupStage
	Status       string
	PausedReason string
	// HeldDay freezes the ramp day while paused (>0 only when status=paused), so a
	// paused schedule holds its current cap exactly regardless of elapsed time.
	// Resume clears it and shifts StartDate so the ramp continues from this day.
	HeldDay   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate normalizes and checks a warmup schedule before persistence. It does
// not verify the VMTA exists (the usecase/FK does); it checks the curve, dates,
// and stage shape.
func (w *WarmupSchedule) Validate() error {
	w.VMTAID = strings.TrimSpace(w.VMTAID)
	w.Curve = strings.ToLower(strings.TrimSpace(w.Curve))
	if w.Status == "" {
		w.Status = WarmupScheduled
	}
	if w.VMTAID == "" {
		return Invalid("WARMUP_VMTA_REQUIRED", "vmta_id is required")
	}
	if w.StartDate.IsZero() {
		return Invalid("WARMUP_START_REQUIRED", "start_date is required")
	}
	w.StartDate = dayStart(w.StartDate)
	if !ValidWarmupStatus(w.Status) {
		return Invalid("WARMUP_STATUS_INVALID", "status %q is not valid", w.Status)
	}
	// M1 ships built-in templates only (custom stage editor is M2): the curve must
	// name a known template, and the stages are resolved from it.
	stages, ok := ResolveWarmupCurve(w.Curve)
	if !ok {
		return Invalid("WARMUP_CURVE_UNKNOWN", "curve %q is not a known template", w.Curve)
	}
	w.Stages = stages
	return ValidWarmupStages(w.Stages)
}

// dayStart normalizes t to a UTC date (midnight), so day math is calendar-based
// and timezone-stable.
func dayStart(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// DayIndex returns the 1-based ramp day for today (day 1 == StartDate). Returns
// 0 or negative before the start date.
func (w *WarmupSchedule) DayIndex(today time.Time) int {
	days := int(dayStart(today).Sub(dayStart(w.StartDate)).Hours() / 24)
	return days + 1
}

// DurationDays returns the last ramp day the curve covers (0 when empty).
func (w *WarmupSchedule) DurationDays() int {
	last := 0
	for _, s := range w.Stages {
		if s.DayTo > last {
			last = s.DayTo
		}
	}
	return last
}

// CapFor returns the messages/day cap for bucket on today and ok=true when a cap
// applies. ok=false before the start date, after the curve completes, or when
// the matched stage sets no positive cap for the bucket. Date-based; pause is
// applied separately via effectiveDay.
func (w *WarmupSchedule) CapFor(bucket string, today time.Time) (int, bool) {
	return w.capForDay(bucket, w.DayIndex(today))
}

// effectiveDay is the ramp day in force now: the frozen HeldDay while paused,
// otherwise the calendar day index.
func (w *WarmupSchedule) effectiveDay(today time.Time) int {
	if w.Status == WarmupPaused && w.HeldDay > 0 {
		return w.HeldDay
	}
	return w.DayIndex(today)
}

// capForDay returns the cap for bucket on a 1-based ramp day, with the default
// bucket as fallback.
func (w *WarmupSchedule) capForDay(bucket string, day int) (int, bool) {
	if day < 1 || day > w.DurationDays() {
		return 0, false
	}
	for _, s := range w.Stages {
		if day < s.DayFrom || day > s.DayTo {
			continue
		}
		n := s.Caps[bucket]
		if n <= 0 {
			n = s.Caps[MBPDefault]
		}
		if n <= 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// appliesToday reports whether this schedule should contribute an egress-path
// cap right now: active or paused (paused holds because the worker freezes
// StartDate), with a non-empty source name. Scheduled/completed contribute none.
func (w *WarmupSchedule) appliesToday() bool {
	return (w.Status == WarmupActive || w.Status == WarmupPaused) && strings.TrimSpace(w.VMTAName) != ""
}

// ResolveWarmupRates turns the active/paused schedules into the per-source,
// per-bucket max_message_rate strings ("N/day") the renderer emits. today is the
// reference date (UTC); only buckets with a cap on that day are included. Pure,
// so the renderer stays deterministic and this is unit-tested directly.
func ResolveWarmupRates(schedules []*WarmupSchedule, today time.Time) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, w := range schedules {
		if w == nil || !w.appliesToday() {
			continue
		}
		day := w.effectiveDay(today)
		for _, bucket := range warmupBuckets {
			cap, ok := w.capForDay(bucket, day)
			if !ok {
				continue
			}
			if out[w.VMTAName] == nil {
				out[w.VMTAName] = map[string]string{}
			}
			out[w.VMTAName][bucket] = strconvDay(cap)
		}
	}
	return out
}

// strconvDay formats a messages/day cap as a KumoMTA throttle spec.
func strconvDay(n int) string {
	return strconv.Itoa(n) + "/day"
}

// mbpDomains maps common receiving domains to their MBP bucket. Curated for the
// big three (the families warmup most needs to pace); everything else falls to
// "default". Rendered into the policy as MBP_BUCKET. Extend as needed.
var mbpDomains = map[string]string{
	"gmail.com":      MBPGmail,
	"googlemail.com": MBPGmail,
	"outlook.com":    MBPMicrosoft,
	"hotmail.com":    MBPMicrosoft,
	"live.com":       MBPMicrosoft,
	"msn.com":        MBPMicrosoft,
	"hotmail.co.uk":  MBPMicrosoft,
	"outlook.co.uk":  MBPMicrosoft,
	"yahoo.com":      MBPYahoo,
	"yahoo.co.uk":    MBPYahoo,
	"ymail.com":      MBPYahoo,
	"rocketmail.com": MBPYahoo,
	"aol.com":        MBPYahoo, // Yahoo/AOL share infrastructure
}

// sortedMBPDomains returns the domain->bucket pairs in domain order (stable
// rendering / checksum).
func sortedMBPDomains() [][2]string {
	keys := make([]string, 0, len(mbpDomains))
	for d := range mbpDomains {
		keys = append(keys, d)
	}
	sort.Strings(keys)
	out := make([][2]string, 0, len(keys))
	for _, d := range keys {
		out = append(out, [2]string{d, mbpDomains[d]})
	}
	return out
}
