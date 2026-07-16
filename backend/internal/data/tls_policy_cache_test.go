package data

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
)

func TestTLSPolicyBlob(t *testing.T) {
	pols := []*biz.TLSPolicy{
		{Domain: "Escom.BG", Mode: biz.TLSModeDisabled, Status: biz.TLSPolicyActive}, // normalized to lower
		{Domain: "secure.example", Mode: biz.TLSModeRequired, Status: biz.TLSPolicyActive},
		{Domain: "off.example", Mode: biz.TLSModeRequired, Status: biz.TLSPolicyDisabled}, // excluded (inactive)
		{Domain: "  ", Mode: biz.TLSModeDisabled, Status: biz.TLSPolicyActive},            // excluded (blank)
		nil,
	}
	blob, err := tlsPolicyBlob(pols)
	if err != nil {
		t.Fatalf("blob: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := map[string]string{"escom.bg": "Disabled", "secure.example": "Required"}
	if len(got) != len(want) {
		t.Fatalf("blob = %s, want %v", blob, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("blob[%q] = %q, want %q (full: %s)", k, got[k], v, blob)
		}
	}
	// The enable_tls values must be KumoMTA's exact strings the policy compares to.
	if got["escom.bg"] != "Disabled" {
		t.Fatalf("disabled mode must render enable_tls=Disabled, got %q", got["escom.bg"])
	}
}

func TestTLSPolicyBlobEmpty(t *testing.T) {
	blob, err := tlsPolicyBlob(nil)
	if err != nil {
		t.Fatalf("blob: %v", err)
	}
	if string(blob) != "{}" {
		t.Fatalf("empty set = %q, want {}", blob)
	}
}

func TestTLSPolicyCacheNilSafe(t *testing.T) {
	var c *TLSPolicyCache // nil
	if c.Enabled() {
		t.Fatal("nil cache must not be Enabled")
	}
	if err := c.Sync(context.Background(), nil); err != nil {
		t.Fatalf("nil cache Sync must be a no-op, got %v", err)
	}
	// A cache with a nil client is also disabled.
	if NewTLSPolicyCache(nil).Enabled() {
		t.Fatal("cache with nil client must not be Enabled")
	}
}
