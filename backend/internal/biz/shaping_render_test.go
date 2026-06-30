package biz

import (
	"strings"
	"testing"
)

func TestRenderBaseShaping(t *testing.T) {
	bps := []*DeliveryBlueprint{
		{Provider: "Gmail", MXPattern: "google.com", ConnRate: "5/min", DeliveriesPerConn: 10, ConnLimit: 3, DailyCap: 150, Status: BlueprintActive},
		{Provider: "Microsoft", MXPattern: "outlook.com", ConnRate: "3/min", DeliveriesPerConn: 10, ConnLimit: 2, DailyCap: 150, Status: BlueprintActive},
		{Provider: "Yahoo", MXPattern: "yahoo.com", Status: BlueprintDisabled}, // skipped
	}
	out := RenderBaseShaping(bps)
	for _, want := range []string{
		`[domain."google.com"]`,
		"mx_rollup = true",
		`max_connection_rate = "5/min"`,
		"max_deliveries_per_connection = 10",
		"connection_limit = 3",
		`max_message_rate = "150/day"`,
		`[domain."outlook.com"]`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("base shaping missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "yahoo.com") {
		t.Fatalf("disabled blueprint must be omitted:\n%s", out)
	}
	// google.com sorts before outlook.com (stable ordering).
	if strings.Index(out, "google.com") > strings.Index(out, "outlook.com") {
		t.Fatal("blocks must be sorted by MX pattern")
	}
}

func TestRenderWarmupShaping(t *testing.T) {
	if got := RenderWarmupShaping(nil); !strings.Contains(got, "none active") {
		t.Fatalf("empty warmup should note none active: %q", got)
	}
	rates := map[string]map[string]string{
		"203.0.113.10": {MBPGmail: "5000/day", MBPDefault: "20000/day"},
	}
	out := RenderWarmupShaping(rates)
	if !strings.Contains(out, `[domain."google.com".sources."203.0.113.10"]`) ||
		!strings.Contains(out, `max_message_rate = "5000/day"`) {
		t.Fatalf("warmup override not emitted:\n%s", out)
	}
	// "default" bucket has no specific MX block.
	if strings.Contains(out, `"20000/day"`) {
		t.Fatalf("default bucket should not emit a domain override:\n%s", out)
	}
}
