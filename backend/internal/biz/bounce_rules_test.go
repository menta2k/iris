package biz

import "testing"

func TestParseEnhancedCode(t *testing.T) {
	cases := map[string]string{
		"550 5.1.1 User unknown":                     "5.1.1",
		"421 4.7.0 Try again later":                  "4.7.0",
		"452 4.2.2 over quota":                       "4.2.2",
		"550 The account does not exist":             "",
		"host said: 421 4.7.0 Delayed rate limiting": "4.7.0",
	}
	for in, want := range cases {
		if got := ParseEnhancedCode(in); got != want {
			t.Errorf("ParseEnhancedCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProviderForDomain(t *testing.T) {
	cases := map[string]string{
		"gmail.com":          "gmail",
		"foo.googlemail.com": "gmail",
		"YAHOO.COM":          "yahoo",
		"outlook.com":        "microsoft",
		"hotmail.com":        "microsoft",
		"example.com":        "",
		"":                   "",
	}
	for in, want := range cases {
		if got := ProviderForDomain(in); got != want {
			t.Errorf("ProviderForDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchBounceRulePriorityAndSpecificity(t *testing.T) {
	rules := DefaultBounceRules()

	// 5.1.1 user-unknown → suppress (Invalid Recipient), beating any broad rule.
	m := MatchBounceRule(rules, BounceSignature{
		SMTPCode: "550", Domain: "gmail.com", Diagnostic: "550 5.1.1 The email account does not exist",
	})
	if m == nil || m.Action != BounceActionSuppress || m.Category != "Invalid Recipient" {
		t.Fatalf("expected suppress/Invalid Recipient, got %+v", m)
	}

	// Gmail rate-limit → throttle (provider + pattern specific).
	m = MatchBounceRule(rules, BounceSignature{
		SMTPCode: "421", Domain: "gmail.com", Diagnostic: "421 4.7.0 our system has detected an unusual rate of unsolicited mail",
	})
	if m == nil || m.Action != BounceActionThrottle {
		t.Fatalf("expected throttle for gmail rate limit, got %+v", m)
	}

	// Generic 421 connection issue → retry.
	m = MatchBounceRule(rules, BounceSignature{
		SMTPCode: "421", Domain: "example.com", Diagnostic: "421 service temporarily unavailable",
	})
	if m == nil || m.Action != BounceActionRetry {
		t.Fatalf("expected retry for generic 421, got %+v", m)
	}

	// No match → nil.
	if got := MatchBounceRule(rules, BounceSignature{SMTPCode: "250", Diagnostic: "250 OK"}); got != nil {
		t.Fatalf("expected no match for 250, got %+v", got)
	}
}

func TestValidateBounceRule(t *testing.T) {
	// Class derived from a 5xx code.
	r := &BounceActionRule{SMTPCode: "550", Action: BounceActionSuppress}
	if err := ValidateBounceRule(r); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if r.Class != BounceClassHard || r.Status != "active" {
		t.Fatalf("expected hard/active, got %+v", r)
	}
	// Empty rule rejected.
	if err := ValidateBounceRule(&BounceActionRule{Action: BounceActionRetry}); err == nil {
		t.Fatal("expected empty-rule error")
	}
	// Bad action rejected.
	if err := ValidateBounceRule(&BounceActionRule{SMTPCode: "421", Action: "nuke"}); err == nil {
		t.Fatal("expected invalid-action error")
	}
	// Bad smtp code rejected.
	if err := ValidateBounceRule(&BounceActionRule{SMTPCode: "99", Action: BounceActionRetry}); err == nil {
		t.Fatal("expected invalid-smtp error")
	}
}

func TestDefaultBounceRulesValid(t *testing.T) {
	for i, r := range DefaultBounceRules() {
		if err := ValidateBounceRule(r); err != nil {
			t.Fatalf("default rule %d invalid: %v (%+v)", i, err, r)
		}
	}
}
