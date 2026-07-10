package biz

import "testing"

func TestTLSPolicyEnableTLSValue(t *testing.T) {
	cases := map[string]string{
		TLSModeRequired:              "Required",
		TLSModeRequiredInsecure:      "RequiredInsecure",
		TLSModeOpportunisticInsecure: "OpportunisticInsecure",
		TLSModeDisabled:              "Disabled",
		"":                           "Required", // unknown/blank defaults to Required
	}
	for mode, want := range cases {
		p := &TLSPolicy{Mode: mode}
		if got := p.EnableTLSValue(); got != want {
			t.Errorf("mode %q → %q, want %q", mode, got, want)
		}
	}
}

func TestTLSPolicyValidate(t *testing.T) {
	valid := []string{TLSModeRequired, TLSModeRequiredInsecure, TLSModeOpportunisticInsecure, TLSModeDisabled}
	for _, m := range valid {
		p := &TLSPolicy{Domain: "example.com", Mode: m}
		if err := p.Validate(); err != nil {
			t.Errorf("mode %q should validate, got %v", m, err)
		}
	}
	// Unknown mode rejected.
	p := &TLSPolicy{Domain: "example.com", Mode: "bogus"}
	if err := p.Validate(); err == nil {
		t.Error("bogus mode should be rejected")
	}
	// Blank mode defaults to required.
	def := &TLSPolicy{Domain: "example.com"}
	if err := def.Validate(); err != nil || def.Mode != TLSModeRequired {
		t.Errorf("blank mode should default to required, got mode=%q err=%v", def.Mode, err)
	}
}
