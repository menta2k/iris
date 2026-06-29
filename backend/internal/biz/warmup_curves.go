package biz

import "sort"

// Built-in warmup curve templates. M1 ships curated curves; a custom stage
// editor is M2. Each curve is a contiguous, ascending set of stages with a
// per-MBP daily cap; after the final day the warmup completes and the cap is
// removed. Caps are deliberately conservative for Microsoft/Yahoo (stricter
// reputation gates) and more generous for the "default" long tail.
const (
	CurveStandard     = "standard"
	CurveConservative = "conservative"
	CurveAggressive   = "aggressive"
)

// st builds a stage; caps are ordered gmail, microsoft, yahoo, default.
func st(from, to, gmail, microsoft, yahoo, def int) WarmupStage {
	return WarmupStage{DayFrom: from, DayTo: to, Caps: map[string]int{
		MBPGmail: gmail, MBPMicrosoft: microsoft, MBPYahoo: yahoo, MBPDefault: def,
	}}
}

// warmupCurves is the registry of built-in templates.
var warmupCurves = map[string][]WarmupStage{
	// ~3 weeks to full volume — the sensible default for most senders.
	CurveStandard: {
		st(1, 2, 50, 50, 50, 200),
		st(3, 4, 100, 100, 100, 500),
		st(5, 6, 500, 300, 300, 1000),
		st(7, 8, 1000, 500, 500, 5000),
		st(9, 11, 5000, 2000, 2000, 20000),
		st(12, 14, 20000, 10000, 10000, 50000),
		st(15, 17, 75000, 40000, 40000, 150000),
		st(18, 21, 200000, 100000, 100000, 500000),
	},
	// ~30 days, smaller steps — for cold IPs or strict audiences.
	CurveConservative: {
		st(1, 3, 20, 20, 20, 100),
		st(4, 6, 50, 50, 50, 200),
		st(7, 9, 100, 100, 100, 500),
		st(10, 13, 500, 300, 300, 2000),
		st(14, 17, 2000, 1000, 1000, 10000),
		st(18, 22, 10000, 5000, 5000, 30000),
		st(23, 27, 40000, 20000, 20000, 100000),
		st(28, 30, 100000, 50000, 50000, 300000),
	},
	// ~12 days — only for warm domains / strong existing reputation.
	CurveAggressive: {
		st(1, 1, 200, 100, 100, 500),
		st(2, 3, 1000, 500, 500, 2000),
		st(4, 5, 5000, 2000, 2000, 20000),
		st(6, 8, 25000, 10000, 10000, 75000),
		st(9, 12, 150000, 75000, 75000, 400000),
	},
}

// ResolveWarmupCurve returns a deep copy of the named template's stages (copied
// so an edited/persisted schedule never aliases the shared template). ok=false
// for an unknown name.
func ResolveWarmupCurve(name string) ([]WarmupStage, bool) {
	stages, ok := warmupCurves[name]
	if !ok {
		return nil, false
	}
	out := make([]WarmupStage, len(stages))
	for i, s := range stages {
		caps := make(map[string]int, len(s.Caps))
		for k, v := range s.Caps {
			caps[k] = v
		}
		out[i] = WarmupStage{DayFrom: s.DayFrom, DayTo: s.DayTo, Caps: caps}
	}
	return out, true
}

// WarmupCurveNames lists the built-in template names (sorted) for the UI.
func WarmupCurveNames() []string {
	names := make([]string, 0, len(warmupCurves))
	for n := range warmupCurves {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ValidWarmupStages checks a curve is non-empty, 1-based, contiguous, and
// ascending (each stage starts right after the previous ends), so day math has
// no gaps or overlaps.
func ValidWarmupStages(stages []WarmupStage) error {
	if len(stages) == 0 {
		return Invalid("WARMUP_STAGES_EMPTY", "warmup curve must have at least one stage")
	}
	want := 1
	for i, s := range stages {
		if s.DayFrom != want {
			return Invalid("WARMUP_STAGES_NONCONTIGUOUS",
				"stage %d must start on day %d (got %d) — stages must be 1-based and contiguous", i+1, want, s.DayFrom)
		}
		if s.DayTo < s.DayFrom {
			return Invalid("WARMUP_STAGES_RANGE", "stage %d has day_to < day_from", i+1)
		}
		hasCap := false
		for _, b := range warmupBuckets {
			if s.Caps[b] < 0 {
				return Invalid("WARMUP_STAGES_NEGATIVE", "stage %d has a negative cap", i+1)
			}
			if s.Caps[b] > 0 {
				hasCap = true
			}
		}
		if !hasCap {
			return Invalid("WARMUP_STAGES_NO_CAP", "stage %d sets no positive cap for any bucket", i+1)
		}
		want = s.DayTo + 1
	}
	return nil
}
