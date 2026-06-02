package service

import (
	"context"
	"testing"
	"time"
)

// fakeRuleSource is a static RuleSource for evaluator + login tests.
type fakeRuleSource struct {
	rows []LoginPolicyRow
	err  error
}

func (f *fakeRuleSource) ListApplicable(_ context.Context, _ *uint32) ([]LoginPolicyRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

// fakeGeo is a static GeoResolver. disabled mimics a nil/missing database
// (indeterminate); err mimics a lookup failure.
type fakeGeo struct {
	code     string
	err      error
	disabled bool
}

func (g *fakeGeo) CountryISO(string) (string, error) {
	if g.disabled {
		return "", nil
	}
	return g.code, g.err
}

// mustTime parses an RFC3339 timestamp for deterministic TIME tests.
func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return ts
}

func TestEvaluateRules(t *testing.T) {
	uid := uint32(1)
	// A Wednesday 14:30 UTC instant for TIME cases.
	wed1430 := mustTime(t, "2026-06-03T14:30:00Z")

	tests := []struct {
		name      string
		rules     []LoginPolicyRow
		attempt   LoginAttempt
		geo       GeoResolver
		wantAllow bool
	}{
		{
			name:      "default allow with no rules",
			rules:     nil,
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			wantAllow: true,
		},
		{
			name:      "blacklist ip match denies",
			rules:     []LoginPolicyRow{{ID: 1, Type: PolicyTypeBlacklist, Method: MethodIP, Value: "1.2.3.0/24"}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			wantAllow: false,
		},
		{
			name:      "blacklist ip no-match allows",
			rules:     []LoginPolicyRow{{ID: 1, Type: PolicyTypeBlacklist, Method: MethodIP, Value: "10.0.0.0/8"}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			wantAllow: true,
		},
		{
			name:      "blacklist bare ip match denies",
			rules:     []LoginPolicyRow{{ID: 1, Type: PolicyTypeBlacklist, Method: MethodIP, Value: "1.2.3.4"}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			wantAllow: false,
		},
		{
			name:      "whitelist ip match allows",
			rules:     []LoginPolicyRow{{ID: 1, Type: PolicyTypeWhitelist, Method: MethodIP, Value: "1.2.3.0/24"}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			wantAllow: true,
		},
		{
			name:      "whitelist ip no-match denies",
			rules:     []LoginPolicyRow{{ID: 1, Type: PolicyTypeWhitelist, Method: MethodIP, Value: "10.0.0.0/8"}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			wantAllow: false,
		},
		{
			name:      "whitelist ip indeterminate (empty client ip) fails open",
			rules:     []LoginPolicyRow{{ID: 1, Type: PolicyTypeWhitelist, Method: MethodIP, Value: "10.0.0.0/8"}},
			attempt:   LoginAttempt{UserID: &uid, IP: "", Now: wed1430},
			wantAllow: true,
		},
		{
			name: "deny precedence: blacklist beats satisfied whitelist",
			rules: []LoginPolicyRow{
				{ID: 1, Type: PolicyTypeWhitelist, Method: MethodIP, Value: "1.2.3.0/24"},
				{ID: 2, Type: PolicyTypeBlacklist, Method: MethodIP, Value: "1.2.3.4"},
			},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			wantAllow: false,
		},
		{
			name: "global + user union: user blacklist denies",
			rules: []LoginPolicyRow{
				{ID: 1, Type: PolicyTypeBlacklist, Method: MethodIP, Value: "9.9.9.9", TargetID: 0},
				{ID: 2, Type: PolicyTypeBlacklist, Method: MethodIP, Value: "1.2.3.4", TargetID: 1},
			},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			wantAllow: false,
		},
		{
			name:      "region whitelist match allows",
			rules:     []LoginPolicyRow{{ID: 1, Type: PolicyTypeWhitelist, Method: MethodRegion, Value: "BG"}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			geo:       &fakeGeo{code: "BG"},
			wantAllow: true,
		},
		{
			name:      "region whitelist no-match denies",
			rules:     []LoginPolicyRow{{ID: 1, Type: PolicyTypeWhitelist, Method: MethodRegion, Value: "BG"}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			geo:       &fakeGeo{code: "US"},
			wantAllow: false,
		},
		{
			name:      "region blacklist match denies",
			rules:     []LoginPolicyRow{{ID: 1, Type: PolicyTypeBlacklist, Method: MethodRegion, Value: "RU"}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			geo:       &fakeGeo{code: "RU"},
			wantAllow: false,
		},
		{
			name:      "region whitelist fails open when geo nil",
			rules:     []LoginPolicyRow{{ID: 1, Type: PolicyTypeWhitelist, Method: MethodRegion, Value: "BG"}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			geo:       nil,
			wantAllow: true,
		},
		{
			name:      "region whitelist fails open on geo error",
			rules:     []LoginPolicyRow{{ID: 1, Type: PolicyTypeWhitelist, Method: MethodRegion, Value: "BG"}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			geo:       &fakeGeo{err: context.DeadlineExceeded},
			wantAllow: true,
		},
		{
			name: "time whitelist inside window allows",
			rules: []LoginPolicyRow{{ID: 1, Type: PolicyTypeWhitelist, Method: MethodTime,
				TimeWindow: &TimeWindow{Start: "09:00", End: "17:00", Timezone: "UTC"}}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			wantAllow: true,
		},
		{
			name: "time whitelist outside window denies",
			rules: []LoginPolicyRow{{ID: 1, Type: PolicyTypeWhitelist, Method: MethodTime,
				TimeWindow: &TimeWindow{Start: "09:00", End: "12:00", Timezone: "UTC"}}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			wantAllow: false,
		},
		{
			name: "time whitelist wrong weekday denies",
			rules: []LoginPolicyRow{{ID: 1, Type: PolicyTypeWhitelist, Method: MethodTime,
				TimeWindow: &TimeWindow{Days: []time.Weekday{time.Monday}, Start: "00:00", End: "23:59", Timezone: "UTC"}}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430}, // Wednesday
			wantAllow: false,
		},
		{
			name: "time blacklist wrap window fails open (unsupported)",
			rules: []LoginPolicyRow{{ID: 1, Type: PolicyTypeBlacklist, Method: MethodTime,
				TimeWindow: &TimeWindow{Start: "22:00", End: "06:00", Timezone: "UTC"}}},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			wantAllow: true,
		},
		{
			name: "independent methods: ip satisfied, region unrestricted",
			rules: []LoginPolicyRow{
				{ID: 1, Type: PolicyTypeWhitelist, Method: MethodIP, Value: "1.2.3.0/24"},
			},
			attempt:   LoginAttempt{UserID: &uid, IP: "1.2.3.4", Now: wed1430},
			geo:       &fakeGeo{code: "US"},
			wantAllow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateRules(tt.rules, tt.attempt, tt.geo)
			if got.Allowed != tt.wantAllow {
				t.Fatalf("evaluateRules allowed=%v reason=%q, want allowed=%v", got.Allowed, got.Reason, tt.wantAllow)
			}
		})
	}
}

func TestLoginFirewallEvaluatePropagatesStoreError(t *testing.T) {
	fw := NewLoginFirewall(&fakeRuleSource{err: context.DeadlineExceeded}, nil)
	res, err := fw.Evaluate(context.Background(), LoginAttempt{IP: "1.2.3.4"})
	if err == nil {
		t.Fatal("expected store error to propagate")
	}
	if !res.Allowed {
		t.Fatal("on store error the result must be fail-open (Allowed=true) so the caller can decide")
	}
}
