package biz

import (
	"strings"
	"testing"
)

func TestBounceRulesToAutomation(t *testing.T) {
	rules := []*BounceActionRule{
		// suspend_domain, all providers → one "default" Suspend block.
		{EnhancedCode: "5.7.1", Action: BounceActionSuspendDomain, ActionConfig: "2h", Status: "active"},
		// throttle, gmail → one SetConfig block per Gmail MX pattern.
		{SMTPCode: "421", Provider: "gmail", Pattern: "rate", Action: BounceActionThrottle, ActionConfig: "max_message_rate=100/h", Status: "active"},
		// retry / suppress / disabled → no shaping.
		{SMTPCode: "421", Action: BounceActionRetry, Status: "active"},
		{EnhancedCode: "5.1.1", Action: BounceActionSuppress, Status: "active"},
		{SMTPCode: "550", Action: BounceActionSuspendDomain, Status: "disabled"},
	}
	auto := BounceRulesToAutomation(rules)

	var suspend, throttle int
	for _, a := range auto {
		if a.Trigger != "2/hr" {
			t.Fatalf("compiled rule must carry the safety threshold trigger, got %q", a.Trigger)
		}
		switch a.Action {
		case AutomationSuspend:
			suspend++
			if a.Domain != "default" || a.Duration != "2h" {
				t.Fatalf("unexpected suspend rule: %+v", a)
			}
		case AutomationSetConfig:
			throttle++
			if a.ConfigName != "max_message_rate" || a.ConfigValue != "100/h" {
				t.Fatalf("unexpected throttle config: %+v", a)
			}
		}
	}
	if suspend != 1 {
		t.Fatalf("expected 1 suspend block, got %d", suspend)
	}
	if throttle != len(providerMXPatterns(MBPGmail)) {
		t.Fatalf("expected one throttle block per Gmail MX pattern (%d), got %d", len(providerMXPatterns(MBPGmail)), throttle)
	}

	// The compiled rules render into valid TSA automation TOML.
	toml := RenderAutomation(auto)
	if !strings.Contains(toml, "action = \"Suspend\"") {
		t.Fatalf("expected a Suspend block in:\n%s", toml)
	}
	if !strings.Contains(toml, "SetConfig") || !strings.Contains(toml, "max_message_rate") {
		t.Fatalf("expected a SetConfig throttle block in:\n%s", toml)
	}
}

func TestBounceRulesToAutomationEmpty(t *testing.T) {
	if got := BounceRulesToAutomation(nil); got != nil {
		t.Fatalf("nil rules should produce no automation, got %+v", got)
	}
	// Only retry/suppress → no automation, so the merged render is unchanged.
	only := []*BounceActionRule{{SMTPCode: "421", Action: BounceActionRetry, Status: "active"}}
	if got := BounceRulesToAutomation(only); len(got) != 0 {
		t.Fatalf("retry-only rules should produce no automation, got %+v", got)
	}
}
