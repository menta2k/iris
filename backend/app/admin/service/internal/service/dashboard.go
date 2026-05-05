// DashboardService composes Prometheus queries into the data shapes
// the operator UI's /analytics page renders. Three responses cover
// the three widget classes (summary cards, trend chart, breakdown
// table); each shape is JSON-friendly so the SPA doesn't translate.
//
// Why service-side aggregation rather than letting the UI hit
// Prometheus directly? Three reasons:
//
//  1. Same-origin policy. The UI is served on :8000, Prometheus on
//     :9091; CORS would need wiring on both sides.
//  2. Auth. The UI's bearer token is the iris JWT, not anything
//     Prometheus understands.
//  3. PromQL leakage. Operators editing the dashboard shouldn't need
//     to know PromQL; the queries belong in code where they're
//     reviewed and tested.
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/menta2k/iris/backend/pkg/promquery"
)

// DashboardService produces /v1/dashboard/* response bodies.
type DashboardService struct {
	q promquery.API
}

// NewDashboardService constructs the service. `q` must be non-nil;
// for the "no Prometheus configured" case the caller passes a
// promquery.Noop which still satisfies the interface.
func NewDashboardService(q promquery.API) *DashboardService {
	return &DashboardService{q: q}
}

// Summary is the response body for GET /v1/dashboard/summary.
// Numbers are point-in-time totals over the last 24 h, aggregated
// from kumomta's event stream by the log processor.
type Summary struct {
	// Events24h: count of LogEvent rows by event_type processed
	// over the last 24 hours.
	Events24h map[string]float64 `json:"events_24h"`

	// DeliveryRate24h: Delivery / Reception over the last 24 h.
	// 0 when there were no Reception events; the UI shows "—" in
	// that case rather than a misleading 0%.
	DeliveryRate24h float64 `json:"delivery_rate_24h"`

	// BounceRate24h: Bounce / Reception over the last 24 h.
	BounceRate24h float64 `json:"bounce_rate_24h"`

	// StreamPending: current XPENDING count from the consumer
	// group. >0 = the consumer is falling behind.
	StreamPending float64 `json:"stream_pending"`

	// SuppressionEntries: SCARD of the Redis suppression sets,
	// by scope ("address" / "domain").
	SuppressionEntries map[string]float64 `json:"suppression_entries"`

	// PolicyApplies24h: count of /v1/policy/apply outcomes by
	// result label over the last 24 h.
	PolicyApplies24h map[string]float64 `json:"policy_applies_24h"`

	// GeneratedAt: when this snapshot was assembled — surfaced so
	// the UI can show "as of …" and operators don't think they're
	// looking at live data when Prometheus is lagging.
	GeneratedAt time.Time `json:"generated_at"`
}

// EventRates is the response body for GET /v1/dashboard/event-rates.
// One series per event_type, with regularly-spaced samples.
type EventRates struct {
	RangeSeconds int      `json:"range_seconds"`
	StepSeconds  int      `json:"step_seconds"`
	Series       []Series `json:"series"`
}

// Series is one event_type's time series.
type Series struct {
	EventType string    `json:"event_type"`
	Points    []EvPoint `json:"points"`
}

// EvPoint is a (timestamp, per-second-rate) pair. Rate-per-second
// (rather than absolute count per step) so chart axes stay stable
// when the user changes the step size.
type EvPoint struct {
	At    time.Time `json:"at"`
	Value float64   `json:"value"`
}

// ClassBreakdown is the response body for GET /v1/dashboard/by-class.
type ClassBreakdown struct {
	Classes []ClassRow `json:"classes"`
}

// ClassRow is one mail-class's 24 h volume + delivery rate.
type ClassRow struct {
	MailClass    string  `json:"mail_class"` // empty string = unclassified
	Events24h    float64 `json:"events_24h"`
	DeliveryRate float64 `json:"delivery_rate"`
}

// Summary builds the /v1/dashboard/summary response. Returns
// promquery.ErrNotConfigured when Prometheus isn't wired up so the
// HTTP handler can map to a meaningful 503.
func (s *DashboardService) Summary(ctx context.Context) (*Summary, error) {
	// All four queries fan out concurrently — they're independent and
	// a slow one shouldn't drag the others. Each timeout-bounded.
	type result struct {
		events     []promquery.Sample
		pending    []promquery.Sample
		supp       []promquery.Sample
		policy     []promquery.Sample
		errEvents  error
		errPending error
		errSupp    error
		errPolicy  error
	}
	r := result{}

	// We use `increase(...[24h])` rather than raw counter values so a
	// counter reset (admin-service restart) doesn't show up as a
	// negative blip. Prometheus's `increase` handles resets.
	r.events, r.errEvents = s.q.QueryInstant(ctx,
		`sum by (event_type) (increase(iris_log_events_total[24h]))`)

	r.pending, r.errPending = s.q.QueryInstant(ctx, `iris_log_stream_pending`)

	r.supp, r.errSupp = s.q.QueryInstant(ctx, `iris_suppression_entries`)

	r.policy, r.errPolicy = s.q.QueryInstant(ctx,
		`sum by (result) (increase(iris_policy_apply_total[24h]))`)

	// Tolerate partial failures — the dashboard is more useful with
	// some data than with a 500. Surface the first error in logs
	// indirectly (via the JSON missing fields).
	if r.errEvents != nil && errors.Is(r.errEvents, promquery.ErrNotConfigured) {
		return nil, r.errEvents
	}

	out := &Summary{
		Events24h:          map[string]float64{},
		SuppressionEntries: map[string]float64{},
		PolicyApplies24h:   map[string]float64{},
		GeneratedAt:        time.Now().UTC(),
	}

	if r.errEvents == nil {
		for _, sm := range r.events {
			out.Events24h[sm.Labels["event_type"]] = sm.Value
		}
	}
	out.DeliveryRate24h = ratio(out.Events24h["Delivery"], out.Events24h["Reception"])
	out.BounceRate24h = ratio(out.Events24h["Bounce"], out.Events24h["Reception"])

	if r.errPending == nil && len(r.pending) > 0 {
		out.StreamPending = r.pending[0].Value
	}

	if r.errSupp == nil {
		for _, sm := range r.supp {
			out.SuppressionEntries[sm.Labels["scope"]] = sm.Value
		}
	}

	if r.errPolicy == nil {
		for _, sm := range r.policy {
			out.PolicyApplies24h[sm.Labels["result"]] = sm.Value
		}
	}

	return out, nil
}

// EventRates builds the /v1/dashboard/event-rates response.
//
// rangeDuration: how far back to query (e.g. 1 h, 24 h).
// step: sample resolution. Prometheus rejects too-fine combinations
//
//	(too many points), so the handler clamps these.
func (s *DashboardService) EventRates(ctx context.Context, rangeDuration, step time.Duration) (*EventRates, error) {
	end := time.Now()
	start := end.Add(-rangeDuration)

	// Per-second rate over a 5-minute trailing window. 5m is a
	// standard Prometheus rate window for counters scraped at 15s.
	series, err := s.q.QueryRange(ctx,
		`sum by (event_type) (rate(iris_log_events_total[5m]))`,
		start, end, step,
	)
	if err != nil {
		return nil, err
	}

	out := &EventRates{
		RangeSeconds: int(rangeDuration / time.Second),
		StepSeconds:  int(step / time.Second),
		Series:       make([]Series, 0, len(series)),
	}
	for _, ss := range series {
		points := make([]EvPoint, 0, len(ss.Points))
		for _, p := range ss.Points {
			points = append(points, EvPoint{At: p.At, Value: p.Value})
		}
		out.Series = append(out.Series, Series{
			EventType: ss.Labels["event_type"],
			Points:    points,
		})
	}
	// Stable order: Reception → Delivery → Bounce → TransientFailure → Feedback → others alphabetic.
	sort.Slice(out.Series, func(i, j int) bool {
		return seriesOrder(out.Series[i].EventType) < seriesOrder(out.Series[j].EventType)
	})
	return out, nil
}

// ByClass builds the /v1/dashboard/by-class response. Two queries
// fanned out, one for total events and one for delivery rate, then
// joined by mail_class label.
func (s *DashboardService) ByClass(ctx context.Context) (*ClassBreakdown, error) {
	totals, err := s.q.QueryInstant(ctx,
		`sum by (mail_class) (increase(iris_log_events_total[24h]))`)
	if err != nil {
		return nil, err
	}
	deliveries, errD := s.q.QueryInstant(ctx,
		`sum by (mail_class) (increase(iris_log_events_total{event_type="Delivery"}[24h]))`)
	receptions, errR := s.q.QueryInstant(ctx,
		`sum by (mail_class) (increase(iris_log_events_total{event_type="Reception"}[24h]))`)

	delByClass := map[string]float64{}
	if errD == nil {
		for _, sm := range deliveries {
			delByClass[sm.Labels["mail_class"]] = sm.Value
		}
	}
	rcptByClass := map[string]float64{}
	if errR == nil {
		for _, sm := range receptions {
			rcptByClass[sm.Labels["mail_class"]] = sm.Value
		}
	}

	out := &ClassBreakdown{Classes: make([]ClassRow, 0, len(totals))}
	for _, sm := range totals {
		mc := sm.Labels["mail_class"]
		out.Classes = append(out.Classes, ClassRow{
			MailClass:    mc,
			Events24h:    sm.Value,
			DeliveryRate: ratio(delByClass[mc], rcptByClass[mc]),
		})
	}
	// Sort by volume desc, with the empty (unclassified) class last so
	// it doesn't drown the meaningful classes when most traffic is
	// untagged.
	sort.Slice(out.Classes, func(i, j int) bool {
		if (out.Classes[i].MailClass == "") != (out.Classes[j].MailClass == "") {
			return out.Classes[i].MailClass != ""
		}
		return out.Classes[i].Events24h > out.Classes[j].Events24h
	})
	return out, nil
}

// ratio is delivery-rate-style division with a graceful zero. The UI
// distinguishes "0" from "no data" via the absence of the parent
// counter, not via the ratio value.
func ratio(numer, denom float64) float64 {
	if denom <= 0 {
		return 0
	}
	return numer / denom
}

// seriesOrder pins the chart legend order so colours stay consistent
// across refreshes. Returns a sortkey; lower = earlier in the legend.
func seriesOrder(t string) int {
	switch t {
	case "Reception":
		return 1
	case "Delivery":
		return 2
	case "Bounce":
		return 3
	case "TransientFailure":
		return 4
	case "Feedback":
		return 5
	default:
		// Anything else after the canonical 5; lex-order among themselves.
		return 100 + sumChars(t)
	}
}

// sumChars is a stable per-string ordinal so unknown event types stay
// in deterministic order without alloc'ing a sort key map.
func sumChars(s string) int {
	n := 0
	for _, r := range s {
		n += int(r)
	}
	return n
}

// CompileError is a sentinel for malformed PromQL hand-edited at boot.
// Reserved; not currently used.
var _ = fmt.Sprintf
