package biz

import (
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestWarmupCurvesResolveAndValidate(t *testing.T) {
	for _, name := range WarmupCurveNames() {
		stages, ok := ResolveWarmupCurve(name)
		if !ok || len(stages) == 0 {
			t.Fatalf("curve %q did not resolve", name)
		}
		if err := ValidWarmupStages(stages); err != nil {
			t.Fatalf("built-in curve %q is invalid: %v", name, err)
		}
		// Resolution must deep-copy: mutating the result must not affect the template.
		stages[0].Caps[MBPGmail] = -999
		again, _ := ResolveWarmupCurve(name)
		if again[0].Caps[MBPGmail] == -999 {
			t.Fatalf("curve %q resolution aliased the shared template", name)
		}
	}
	if _, ok := ResolveWarmupCurve("nope"); ok {
		t.Fatal("unknown curve must return ok=false")
	}
}

func TestValidWarmupStages(t *testing.T) {
	good := []WarmupStage{st(1, 2, 10, 10, 10, 50), st(3, 4, 100, 100, 100, 200)}
	if err := ValidWarmupStages(good); err != nil {
		t.Fatalf("valid stages rejected: %v", err)
	}
	cases := map[string][]WarmupStage{
		"WARMUP_STAGES_EMPTY":         {},
		"WARMUP_STAGES_NONCONTIGUOUS": {st(1, 2, 10, 10, 10, 10), st(4, 5, 20, 20, 20, 20)}, // gap (day 3 missing)
		"WARMUP_STAGES_RANGE":         {{DayFrom: 1, DayTo: 0, Caps: map[string]int{MBPDefault: 10}}},
		"WARMUP_STAGES_NO_CAP":        {st(1, 2, 0, 0, 0, 0)},
		"WARMUP_STAGES_NEGATIVE":      {st(1, 2, -1, 0, 0, 10)},
	}
	for wantReason, stages := range cases {
		assertReason(t, ValidWarmupStages(stages), wantReason)
	}
}

func TestWarmupCapFor(t *testing.T) {
	w := &WarmupSchedule{StartDate: date(2026, 6, 1)}
	w.Stages, _ = ResolveWarmupCurve(CurveStandard)

	if w.DurationDays() != 21 {
		t.Fatalf("standard curve duration = %d, want 21", w.DurationDays())
	}
	type tc struct {
		day    time.Time
		bucket string
		want   int
		ok     bool
	}
	for _, c := range []tc{
		{date(2026, 5, 31), MBPGmail, 0, false},         // before start
		{date(2026, 6, 1), MBPGmail, 50, true},          // day 1
		{date(2026, 6, 1), MBPDefault, 200, true},       // day 1 default
		{date(2026, 6, 10), MBPGmail, 5000, true},       // day 10 (stage 9-11)
		{date(2026, 6, 21), MBPMicrosoft, 100000, true}, // day 21 (last)
		{date(2026, 6, 22), MBPGmail, 0, false},         // day 22 → completed
	} {
		got, ok := w.CapFor(c.bucket, c.day)
		if got != c.want || ok != c.ok {
			t.Fatalf("CapFor(%s, %s) = (%d,%v), want (%d,%v)", c.bucket, c.day.Format("2006-01-02"), got, ok, c.want, c.ok)
		}
	}

	// default fallback: a stage with only a default cap applies to every bucket.
	wd := &WarmupSchedule{StartDate: date(2026, 6, 1), Stages: []WarmupStage{{DayFrom: 1, DayTo: 3, Caps: map[string]int{MBPDefault: 99}}}}
	if got, ok := wd.CapFor(MBPGmail, date(2026, 6, 2)); !ok || got != 99 {
		t.Fatalf("default fallback: got (%d,%v), want (99,true)", got, ok)
	}
}

func TestResolveWarmupRates(t *testing.T) {
	mk := func(status string) *WarmupSchedule {
		s := &WarmupSchedule{VMTAName: "v1", VMTAID: "id1", StartDate: date(2026, 6, 1), Status: status}
		s.Stages, _ = ResolveWarmupCurve(CurveStandard)
		return s
	}
	today := date(2026, 6, 1) // day 1: gmail/ms/yahoo=50, default=200

	// active → rates present, formatted as N/day, in the canonical buckets.
	rates := ResolveWarmupRates([]*WarmupSchedule{mk(WarmupActive)}, today)
	if rates["v1"][MBPGmail] != "50/day" || rates["v1"][MBPDefault] != "200/day" {
		t.Fatalf("active rates wrong: %+v", rates["v1"])
	}
	// paused → still applies (held cap).
	if r := ResolveWarmupRates([]*WarmupSchedule{mk(WarmupPaused)}, today); r["v1"][MBPGmail] != "50/day" {
		t.Fatalf("paused should hold a cap: %+v", r)
	}
	// scheduled / completed / no-name → contribute nothing.
	for _, s := range []*WarmupSchedule{mk(WarmupScheduled), mk(WarmupCompleted)} {
		if r := ResolveWarmupRates([]*WarmupSchedule{s}, today); len(r) != 0 {
			t.Fatalf("status %s should yield no rates: %+v", s.Status, r)
		}
	}
	nameless := mk(WarmupActive)
	nameless.VMTAName = ""
	if r := ResolveWarmupRates([]*WarmupSchedule{nameless}, today); len(r) != 0 {
		t.Fatalf("nameless schedule should yield no rates: %+v", r)
	}
	// before start: active but day 0 → no rate.
	if r := ResolveWarmupRates([]*WarmupSchedule{mk(WarmupActive)}, date(2026, 5, 20)); len(r) != 0 {
		t.Fatalf("pre-start should yield no rates: %+v", r)
	}
}
