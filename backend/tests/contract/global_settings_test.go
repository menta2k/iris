package contract

import (
	"strings"
	"testing"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
)

// TestGlobalSettingsDrivesPolicy verifies the end-to-end UI path: updating the
// global settings (rspamd enforce) through the API changes what the KumoMTA
// config generator emits — without any config-file change.
func TestGlobalSettingsDrivesPolicy(t *testing.T) {
	svc := newService(t)
	ctx := ownerCtx()

	// A hosted domain (DKIM) so rspamd has something to scope to.
	if _, err := svc.CreateDKIMDomain(ctx, &adminv1.CreateDKIMDomainRequest{Domain: "hosted.example", Selector: "s1"}); err != nil {
		t.Fatalf("CreateDKIMDomain: %v", err)
	}

	// Initially rspamd is off — the policy emits the disabled stub.
	before, err := svc.GenerateKumoConfig(ctx, &adminv1.GenerateKumoConfigRequest{})
	if err != nil {
		t.Fatalf("generate before: %v", err)
	}
	if !strings.Contains(before.GetContent(), "rspamd): disabled") {
		t.Fatalf("expected rspamd disabled before update:\n%s", before.GetContent())
	}

	// Update settings via the API.
	got, err := svc.UpdateGlobalSettings(ctx, &adminv1.UpdateGlobalSettingsRequest{
		RspamdMode: "enforce", RspamdUrl: "http://rspamd:11334",
		LogStreamRedisUrl: "redis://redis:6379",
	})
	if err != nil {
		t.Fatalf("UpdateGlobalSettings: %v", err)
	}
	if got.GetRspamdMode() != "enforce" {
		t.Fatalf("unexpected settings echo: %+v", got)
	}

	// Get round-trips the stored value.
	read, err := svc.GetGlobalSettings(ctx, &adminv1.GetGlobalSettingsRequest{})
	if err != nil {
		t.Fatalf("GetGlobalSettings: %v", err)
	}
	if read.GetRspamdMode() != "enforce" || read.GetRspamdUrl() != "http://rspamd:11334" {
		t.Fatalf("settings did not persist: %+v", read)
	}

	// The generated policy now reflects the UI change.
	after, err := svc.GenerateKumoConfig(ctx, &adminv1.GenerateKumoConfigRequest{})
	if err != nil {
		t.Fatalf("generate after: %v", err)
	}
	if !after.GetValid() {
		t.Fatalf("policy should lint valid, issues: %v", after.GetLintIssues())
	}
	if !strings.Contains(after.GetContent(), "RSPAMD_ENFORCE = true") ||
		!strings.Contains(after.GetContent(), "/checkv2") ||
		!strings.Contains(after.GetContent(), "configure_log_hook") {
		t.Fatalf("UI settings did not flow into the policy:\n%s", after.GetContent())
	}
}

// TestUnauthorizedSettingsWriteDenied verifies the write permission is enforced.
func TestUnauthorizedSettingsWriteDenied(t *testing.T) {
	svc := newService(t)
	if _, err := svc.UpdateGlobalSettings(t.Context(), &adminv1.UpdateGlobalSettingsRequest{RspamdMode: "off"}); err == nil {
		t.Fatal("expected unauthenticated rejection")
	}
}
