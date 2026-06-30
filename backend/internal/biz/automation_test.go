package biz

import (
	"strings"
	"testing"
)

func TestAutomationValidate(t *testing.T) {
	ok := &AutomationRule{Domain: "comcast.net", Regex: "RL0000", Action: AutomationSetConfig,
		ConfigName: "max_connection_rate", ConfigValue: "10000/h", Trigger: "2/hr", Duration: "2 hours"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	suspend := &AutomationRule{Domain: "default", Regex: "deferred", Action: AutomationSuspend, Duration: "90m"}
	if err := suspend.Validate(); err != nil {
		t.Fatalf("valid suspend rejected: %v", err)
	}
	if suspend.Trigger != "immediate" {
		t.Fatalf("trigger should default to immediate, got %q", suspend.Trigger)
	}

	cases := map[string]*AutomationRule{
		"AUTOMATION_DOMAIN_INVALID":        {Domain: "not a domain", Regex: "x", Action: AutomationSuspend, Duration: "1h"},
		"AUTOMATION_REGEX_REQUIRED":        {Domain: "gmail.com", Action: AutomationSuspend, Duration: "1h"},
		"AUTOMATION_ACTION_INVALID":        {Domain: "gmail.com", Regex: "x", Action: "bogus", Duration: "1h"},
		"AUTOMATION_CONFIG_NAME_INVALID":   {Domain: "gmail.com", Regex: "x", Action: AutomationSetConfig, ConfigName: "nope", ConfigValue: "1/h", Duration: "1h"},
		"AUTOMATION_CONFIG_VALUE_REQUIRED": {Domain: "gmail.com", Regex: "x", Action: AutomationSetConfig, ConfigName: "max_message_rate", Duration: "1h"},
		"AUTOMATION_TRIGGER_INVALID":       {Domain: "gmail.com", Regex: "x", Action: AutomationSuspend, Trigger: "sometimes", Duration: "1h"},
		"AUTOMATION_DURATION_INVALID":      {Domain: "gmail.com", Regex: "x", Action: AutomationSuspend, Duration: "soon"},
	}
	for reason, r := range cases {
		assertReason(t, r.Validate(), reason)
	}
}

func TestRenderAutomation(t *testing.T) {
	rules := []*AutomationRule{
		{Domain: "comcast.net", Regex: "RL0000", Action: AutomationSetConfig, ConfigName: "max_connection_rate", ConfigValue: "10000/h", Trigger: "2/hr", Duration: "2 hours", Status: AutomationActive},
		{Domain: "yahoo.com", Regex: `\[TSS04\]`, Action: AutomationSuspend, Trigger: "immediate", Duration: "2 hours", Status: AutomationActive},
		{Domain: "gmail.com", Regex: "x", Action: AutomationSuspend, Duration: "1h", Status: AutomationDisabled}, // skipped
	}
	out := RenderAutomation(rules)
	for _, want := range []string{
		`[["comcast.net".automation]]`,
		`regex = '''RL0000'''`,
		`action = {SetConfig={name="max_connection_rate", value="10000/h"}}`,
		`trigger = {Threshold="2/hr"}`,
		`duration = "2 hours"`,
		`[["yahoo.com".automation]]`,
		`action = "Suspend"`,
		`trigger = "Immediate"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("automation render missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "gmail.com") {
		t.Fatalf("disabled rule must be omitted:\n%s", out)
	}
}
