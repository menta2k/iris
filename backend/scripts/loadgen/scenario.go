// Package main — scenario.go: YAML model for the e2e test harness.
//
// A scenario file describes the full state to seed (VMTAs, groups, classes,
// routing rules, FBL test routes), the traffic patterns to drive, and the
// post-run assertions. Designed to be readable/editable by hand — see
// deploy/test/scenarios/mixed.yaml for an example.
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Scenario is the top-level YAML document. Each section is optional so a
// minimal smoke-test scenario can be three lines.
type Scenario struct {
	Setup     SetupBlock      `yaml:"setup"`
	Scenarios []TrafficBlock  `yaml:"scenarios"`
	Assert    AssertionBlock  `yaml:"assert"`
}

// SetupBlock is the desired state we'll bring up via REST before traffic
// runs. Existing resources with the same name are reused (idempotent).
type SetupBlock struct {
	VMTAs       []SetupVMTA       `yaml:"vmtas"`
	Groups      []SetupGroup      `yaml:"groups"`
	Classes     []SetupMailClass  `yaml:"classes"`
	Rules       []SetupRule       `yaml:"rules"`
	TestRoutes  map[string]string `yaml:"test_routes"`
	Suppressions []SetupSuppress  `yaml:"suppressions"`
}

type SetupVMTA struct {
	Name       string   `yaml:"name"`
	HeloName   string   `yaml:"ehlo"`
	SourceIPs  []string `yaml:"source_ips"`
	MaxConn    uint32   `yaml:"max_connections,omitempty"`
}

type SetupGroup struct {
	Name    string             `yaml:"name"`
	Members []SetupGroupMember `yaml:"members"`
}

type SetupGroupMember struct {
	Vmta   string `yaml:"vmta"`
	Weight uint32 `yaml:"weight"`
}

type SetupMailClass struct {
	Name       string `yaml:"name"`
	TargetKind string `yaml:"target_kind"` // "vmta" | "vmta_group"
	TargetRef  string `yaml:"target_ref"`
}

type SetupRule struct {
	Name      string         `yaml:"name"`
	Priority  int32          `yaml:"priority"`
	When      map[string]any `yaml:"when"` // simple field=value (one key)
	TargetK   string         `yaml:"target_kind"`
	TargetRef string         `yaml:"target_ref"`
}

type SetupSuppress struct {
	Address string `yaml:"address"`
	Scope   string `yaml:"scope"`
	Reason  string `yaml:"reason"`
}

// TrafficBlock describes one bursty submission pattern. Either RatePerSec +
// DurationSec, or Total (one-shot serial), is required.
type TrafficBlock struct {
	Name        string            `yaml:"name"`
	From        string            `yaml:"from"`
	To          string            `yaml:"to"`
	Headers     map[string]string `yaml:"headers,omitempty"`
	RatePerSec  int               `yaml:"rate_per_sec,omitempty"`
	DurationSec int               `yaml:"duration_sec,omitempty"`
	Total       int               `yaml:"total,omitempty"`
	BodyBytes   int               `yaml:"body_bytes,omitempty"`
}

// AssertionBlock is checked after traffic drains. Counts use ">= N" or "== N"
// notation; specific keys are tolerant — missing means "don't check".
type AssertionBlock struct {
	LogEvent           map[string]string `yaml:"log_event"`            // event_type → "op N"
	FeedbackReports    string            `yaml:"feedback_reports"`     // "op N"
	SuppressionEntries map[string]string `yaml:"suppression_entries"`  // reason → "op N"
	StreamDrainTimeout int               `yaml:"redis_stream_drained_within_sec,omitempty"`
}

// LoadScenario reads a YAML file and validates the minimum-required fields.
func LoadScenario(path string) (*Scenario, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario: %w", err)
	}
	var s Scenario
	if err := yaml.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("parse scenario: %w", err)
	}
	for i, t := range s.Scenarios {
		if t.Name == "" {
			return nil, fmt.Errorf("scenarios[%d]: name required", i)
		}
		if t.From == "" || t.To == "" {
			return nil, fmt.Errorf("scenarios[%d] %s: from + to required", i, t.Name)
		}
		if t.Total == 0 && (t.RatePerSec == 0 || t.DurationSec == 0) {
			return nil, fmt.Errorf("scenarios[%d] %s: either total or (rate_per_sec + duration_sec) required", i, t.Name)
		}
	}
	return &s, nil
}
