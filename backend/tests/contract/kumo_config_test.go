package contract

import (
	"strings"
	"testing"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
)

// TestKumoConfigContract exercises generate + apply through the Service: it
// creates outbound configuration, generates the KumoMTA policy, asserts the
// rendered content reflects the config, and applies it with confirmation.
func TestKumoConfigContract(t *testing.T) {
	svc := newService(t)
	ctx := ownerCtx()

	lid := seedListener(t, svc, "k-listener", "203.0.113.80")
	a, err := svc.CreateVMTA(ctx, &adminv1.CreateVMTARequest{Name: "k-a", IpAddress: "203.0.113.81", EhloName: "k-a.example.com", ListenerId: lid})
	if err != nil {
		t.Fatalf("CreateVMTA: %v", err)
	}
	g, err := svc.CreateVMTAGroups(ctx, &adminv1.CreateVMTAGroupRequest{
		Name: "k-pool", Members: []*adminv1.VMTAGroupMember{{VmtaId: a.GetId(), Weight: 10}},
	})
	if err != nil {
		t.Fatalf("CreateVMTAGroups: %v", err)
	}
	if _, err := svc.CreateRoutingRule(ctx, &adminv1.CreateRoutingRuleRequest{
		Name: "k-route", MatchType: "mailclass", MatchValue: "bulk", Priority: 100,
		TargetType: "vmta_group", TargetId: g.GetId(),
	}); err != nil {
		t.Fatalf("CreateRoutingRule: %v", err)
	}
	// Suppressions are enforced via Redis (not rendered into the policy), so the
	// create exercises the API but is not expected to appear in the generated Lua.
	if _, err := svc.CreateSuppression(ctx, &adminv1.CreateSuppressionRequest{Type: "domain", Value: "blocked.example"}); err != nil {
		t.Fatalf("CreateSuppression: %v", err)
	}

	cfg, err := svc.GenerateKumoConfig(ctx, &adminv1.GenerateKumoConfigRequest{})
	if err != nil {
		t.Fatalf("GenerateKumoConfig: %v", err)
	}
	if cfg.GetVmtaCount() != 1 || cfg.GetPoolCount() != 1 || cfg.GetRouteCount() != 1 {
		t.Fatalf("unexpected counts: %+v", cfg)
	}
	if !cfg.GetValid() {
		t.Fatalf("rendered policy should be valid Lua, issues: %v", cfg.GetLintIssues())
	}
	if !strings.Contains(cfg.GetContent(), `SOURCES["k-a"]`) ||
		!strings.Contains(cfg.GetContent(), `POOLS["k-pool"]`) {
		t.Fatalf("rendered policy missing expected entries:\n%s", cfg.GetContent())
	}
	if cfg.GetChecksum() == "" {
		t.Fatal("expected a checksum")
	}

	// Apply requires confirmation.
	if _, err := svc.ApplyKumoConfig(ctx, &adminv1.ApplyKumoConfigRequest{}); err == nil {
		t.Fatal("expected confirmation-required error")
	}

	res, err := svc.ApplyKumoConfig(ctx, &adminv1.ApplyKumoConfigRequest{ConfirmationId: "apply-1"})
	if err != nil {
		t.Fatalf("ApplyKumoConfig: %v", err)
	}
	if res.GetStatus() != "succeeded" || res.GetChecksum() != cfg.GetChecksum() {
		t.Fatalf("unexpected apply result: %+v", res)
	}
}

// TestKumoConfigDriftStatus verifies the drift indicator: configuration changes
// flag drift until the config is applied, and a further change re-flags it.
func TestKumoConfigDriftStatus(t *testing.T) {
	svc := newService(t)
	ctx := ownerCtx()

	// Nothing applied yet → drift (never applied).
	st, err := svc.KumoConfigStatus(ctx, &adminv1.KumoConfigStatusRequest{})
	if err != nil {
		t.Fatalf("KumoConfigStatus: %v", err)
	}
	if !st.GetDrift() || !st.GetNeverApplied() {
		t.Fatalf("expected drift + never-applied initially, got %+v", st)
	}

	// A listener is part of the init block, so this change requires a restart.
	lid := seedListener(t, svc, "d-listener", "203.0.113.81")
	if _, err := svc.CreateVMTA(ctx, &adminv1.CreateVMTARequest{Name: "d-a", IpAddress: "203.0.113.82", EhloName: "d-a.example.com", ListenerId: lid}); err != nil {
		t.Fatalf("CreateVMTA: %v", err)
	}
	st, err = svc.KumoConfigStatus(ctx, &adminv1.KumoConfigStatusRequest{})
	if err != nil {
		t.Fatalf("KumoConfigStatus after listener: %v", err)
	}
	if !st.GetDrift() || !st.GetRestartRequired() {
		t.Fatalf("a listener change must require restart, got %+v", st)
	}

	// Apply → no drift. The apply restarted (init change).
	res, err := svc.ApplyKumoConfig(ctx, &adminv1.ApplyKumoConfigRequest{ConfirmationId: "drift-1"})
	if err != nil {
		t.Fatalf("ApplyKumoConfig: %v", err)
	}
	if !res.GetRestarted() {
		t.Fatalf("applying a listener (init) change should report restarted, got %+v", res)
	}
	st, err = svc.KumoConfigStatus(ctx, &adminv1.KumoConfigStatusRequest{})
	if err != nil {
		t.Fatalf("KumoConfigStatus after apply: %v", err)
	}
	if st.GetDrift() || st.GetNeverApplied() || st.GetRestartRequired() {
		t.Fatalf("expected no drift after apply, got %+v", st)
	}

	// A VMTA-only change drifts but is reload-safe (init block unchanged).
	if _, err := svc.CreateVMTA(ctx, &adminv1.CreateVMTARequest{Name: "d-b", IpAddress: "203.0.113.83", EhloName: "d-b.example.com", ListenerId: lid}); err != nil {
		t.Fatalf("CreateVMTA 2: %v", err)
	}
	st, err = svc.KumoConfigStatus(ctx, &adminv1.KumoConfigStatusRequest{})
	if err != nil {
		t.Fatalf("KumoConfigStatus after change: %v", err)
	}
	if !st.GetDrift() {
		t.Fatalf("expected drift after a new VMTA, got %+v", st)
	}
	if st.GetRestartRequired() {
		t.Fatalf("a VMTA-only change should be reload-safe, got restart_required: %+v", st)
	}
}
