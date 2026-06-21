package contract

import (
	"testing"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
)

// TestOutboundContract exercises the VMTA, group, and routing handlers end to
// end through the Service, asserting the proto response shapes (T034).
func TestOutboundContract(t *testing.T) {
	svc := newService(t)
	ctx := ownerCtx()

	la := seedListener(t, svc, "lst-a", "203.0.113.60")
	lb := seedListener(t, svc, "lst-b", "203.0.113.61")
	a, err := svc.CreateVMTA(ctx, &adminv1.CreateVMTARequest{Name: "a", ListenerId: la})
	if err != nil {
		t.Fatalf("CreateVMTA: %v", err)
	}
	if a.GetId() == "" || a.GetStatus() != "active" {
		t.Fatalf("unexpected VMTA reply: %+v", a)
	}
	b, err := svc.CreateVMTA(ctx, &adminv1.CreateVMTARequest{Name: "b", ListenerId: lb})
	if err != nil {
		t.Fatalf("CreateVMTA b: %v", err)
	}

	group, err := svc.CreateVMTAGroups(ctx, &adminv1.CreateVMTAGroupRequest{
		Name: "pool", Members: []*adminv1.VMTAGroupMember{
			{VmtaId: a.GetId(), Weight: 70}, {VmtaId: b.GetId(), Weight: 30},
		},
	})
	if err != nil {
		t.Fatalf("CreateVMTAGroups: %v", err)
	}
	if len(group.GetMembers()) != 2 {
		t.Fatalf("expected 2 members, got %d", len(group.GetMembers()))
	}

	rule, err := svc.CreateRoutingRule(ctx, &adminv1.CreateRoutingRuleRequest{
		Name: "r", MatchType: "mailclass", MatchValue: "bulk", Priority: 100,
		TargetType: "vmta_group", TargetId: group.GetId(),
	})
	if err != nil {
		t.Fatalf("CreateRoutingRule: %v", err)
	}
	if rule.GetTargetId() != group.GetId() {
		t.Fatalf("routing rule target mismatch: %+v", rule)
	}

	vmtas, err := svc.ListVMTAs(ctx, &adminv1.ListVMTAsRequest{})
	if err != nil {
		t.Fatalf("ListVMTAs: %v", err)
	}
	if len(vmtas.GetItems()) != 2 {
		t.Fatalf("expected 2 vmtas listed, got %d", len(vmtas.GetItems()))
	}

	rules, err := svc.ListRoutingRules(ctx, &adminv1.ListRoutingRulesRequest{MatchType: "mailclass"})
	if err != nil {
		t.Fatalf("ListRoutingRules: %v", err)
	}
	if len(rules.GetItems()) != 1 {
		t.Fatalf("expected 1 rule listed, got %d", len(rules.GetItems()))
	}
}

// TestOutboundContractValidation asserts invalid input is rejected by the handler.
func TestOutboundContractValidation(t *testing.T) {
	svc := newService(t)
	if _, err := svc.CreateVMTA(ownerCtx(), &adminv1.CreateVMTARequest{Name: ""}); err == nil {
		t.Fatal("expected validation error from CreateVMTA")
	}
}
