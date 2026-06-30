package biz

import "testing"

func TestBlueprintValidate(t *testing.T) {
	ok := &DeliveryBlueprint{Provider: "Gmail", MXPattern: "google.com", ConnRate: "5/min", DeliveriesPerConn: 10, ConnLimit: 3, DailyCap: 150}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid blueprint rejected: %v", err)
	}
	if ok.Status != BlueprintActive {
		t.Fatalf("status should default to active, got %q", ok.Status)
	}

	cases := map[string]*DeliveryBlueprint{
		"BLUEPRINT_PROVIDER_REQUIRED": {MXPattern: "google.com"},
		"BLUEPRINT_MX_INVALID":        {Provider: "Gmail", MXPattern: "not a domain"},
		"BLUEPRINT_CONN_RATE_INVALID": {Provider: "Gmail", MXPattern: "google.com", ConnRate: "5 per minute"},
		"BLUEPRINT_STATUS_INVALID":    {Provider: "Gmail", MXPattern: "google.com", Status: "bogus"},
		"BLUEPRINT_DAILY_CAP_RANGE":   {Provider: "Gmail", MXPattern: "google.com", DailyCap: -1},
		"BLUEPRINT_CONN_LIMIT_RANGE":  {Provider: "Gmail", MXPattern: "google.com", ConnLimit: -1},
	}
	for reason, b := range cases {
		assertReason(t, b.Validate(), reason)
	}
}

func TestDefaultBlueprintsValid(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range DefaultBlueprints() {
		bb := b
		if err := bb.Validate(); err != nil {
			t.Fatalf("default blueprint %s invalid: %v", b.MXPattern, err)
		}
		if seen[b.MXPattern] {
			t.Fatalf("duplicate default mx_pattern %s", b.MXPattern)
		}
		seen[b.MXPattern] = true
	}
}
