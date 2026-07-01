package biz

import (
	"regexp"
	"strings"
	"time"
)

// AutomationRule is an operator-authored KumoMTA Traffic Shaping Automation rule:
// when the SMTP response from a destination matches Regex (optionally only after
// a Trigger threshold of matches), apply Action for Duration. iris renders these
// into the base shaping file as [["<domain>".automation]] blocks, which the TSA
// daemon evaluates against live delivery events and the result is layered under
// the IP-warmup ceiling on the kumod side.
type AutomationRule struct {
	ID     string
	Domain string // receiving MX pattern the rule applies to, or "default"
	Regex  string // SMTP response pattern (a Rust regex)
	Action string // suspend | suspend_tenant | set_config
	// ConfigName / ConfigValue apply only when Action is set_config: the egress
	// path key to override (e.g. max_message_rate) and its value (e.g. "100/h").
	ConfigName  string
	ConfigValue string
	Trigger     string // "immediate" or a threshold rate like "2/hr"
	Duration    string // how long the action holds, e.g. "2 hours"
	Status      string // active | disabled
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Automation action values.
const (
	AutomationSuspend       = "suspend"        // Suspend the ready queue for the domain
	AutomationSuspendTenant = "suspend_tenant" // SuspendTenant
	AutomationSetConfig     = "set_config"     // SetConfig{name,value}: tighten a limit
)

// Automation statuses (reuse blueprint status values).
const (
	AutomationActive   = "active"
	AutomationDisabled = "disabled"
)

// automationConfigKeys are the egress-path limits a set_config action may tighten.
var automationConfigKeys = map[string]bool{
	"max_message_rate":              true,
	"max_connection_rate":           true,
	"connection_limit":              true,
	"max_deliveries_per_connection": true,
}

// durationRe matches a KumoMTA-style duration like "90m", "2 hours", "5 minutes".
var durationRe = regexp.MustCompile(`^\d+\s*(s|sec|second|seconds|m|min|minute|minutes|h|hr|hour|hours|d|day|days)$`)

// triggerRateRe matches a threshold rate like "2/hr" or "10/min".
var triggerRateRe = regexp.MustCompile(`^[1-9][0-9]*/(s|m|min|h|hr|hour|d|day)$`)

// ValidAutomationAction reports whether a is a known action.
func ValidAutomationAction(a string) bool {
	switch a {
	case AutomationSuspend, AutomationSuspendTenant, AutomationSetConfig:
		return true
	default:
		return false
	}
}

// Validate normalizes and checks an automation rule before persistence.
func (r *AutomationRule) Validate() error {
	r.Domain = strings.ToLower(strings.TrimSpace(r.Domain))
	r.Regex = strings.TrimSpace(r.Regex)
	r.Action = strings.ToLower(strings.TrimSpace(r.Action))
	r.ConfigName = strings.ToLower(strings.TrimSpace(r.ConfigName))
	r.ConfigValue = strings.TrimSpace(r.ConfigValue)
	r.Trigger = strings.ToLower(strings.TrimSpace(r.Trigger))
	r.Duration = strings.TrimSpace(r.Duration)
	if r.Trigger == "" {
		r.Trigger = "immediate"
	}
	if r.Status == "" {
		r.Status = AutomationActive
	}

	if r.Domain != "default" && (len(r.Domain) > 253 || !dnsNameRe.MatchString(r.Domain)) {
		return Invalid("AUTOMATION_DOMAIN_INVALID", "domain %q is not a valid domain (or \"default\")", r.Domain)
	}
	if r.Regex == "" {
		return Invalid("AUTOMATION_REGEX_REQUIRED", "regex is required")
	}
	if strings.Contains(r.Regex, "'''") {
		return Invalid("AUTOMATION_REGEX_INVALID", "regex must not contain a triple quote")
	}
	if !ValidAutomationAction(r.Action) {
		return Invalid("AUTOMATION_ACTION_INVALID", "action %q is not valid", r.Action)
	}
	if r.Action == AutomationSetConfig {
		if !automationConfigKeys[r.ConfigName] {
			return Invalid("AUTOMATION_CONFIG_NAME_INVALID", "config_name %q is not a tunable limit", r.ConfigName)
		}
		if r.ConfigValue == "" {
			return Invalid("AUTOMATION_CONFIG_VALUE_REQUIRED", "config_value is required for set_config")
		}
	}
	if r.Trigger != "immediate" && !triggerRateRe.MatchString(r.Trigger) {
		return Invalid("AUTOMATION_TRIGGER_INVALID", "trigger must be \"immediate\" or a rate like 2/hr")
	}
	if r.Duration == "" || !durationRe.MatchString(r.Duration) {
		return Invalid("AUTOMATION_DURATION_INVALID", "duration %q must be like \"2 hours\" or \"90m\"", r.Duration)
	}
	if r.Status != AutomationActive && r.Status != AutomationDisabled {
		return Invalid("AUTOMATION_STATUS_INVALID", "status %q is not valid", r.Status)
	}
	return nil
}
