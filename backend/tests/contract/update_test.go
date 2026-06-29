package contract

import (
	"strings"
	"testing"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// TestEditOutboundEntities verifies the edit (update) path for VMTAs, VMTA
// groups, and routing rules through the Service.
func TestEditOutboundEntities(t *testing.T) {
	svc := newService(t)
	ctx := ownerCtx()

	la := seedListener(t, svc, "lst-edit-a", "203.0.113.90")
	lb := seedListener(t, svc, "lst-edit-b", "203.0.113.91")
	a, err := svc.CreateVMTA(ctx, &adminv1.CreateVMTARequest{Name: "edit-a", IpAddress: "203.0.113.61", EhloName: "edit-a.example.com", ListenerId: la})
	if err != nil {
		t.Fatalf("create vmta: %v", err)
	}

	// Edit the VMTA: change listener, max connections, status, notes.
	upd, err := svc.UpdateVMTA(ctx, &adminv1.UpdateVMTARequest{
		Id: a.GetId(), Name: "edit-a", ListenerId: lb, MaxConnections: 25,
		Status: "disabled", Notes: "drained for maint",
	})
	if err != nil {
		t.Fatalf("update vmta: %v", err)
	}
	if upd.GetListenerId() != lb || upd.GetMaxConnections() != 25 || upd.GetStatus() != "disabled" || upd.GetNotes() != "drained for maint" {
		t.Fatalf("vmta edit did not apply: %+v", upd)
	}

	// A second VMTA + a group, then edit the group's membership/weights.
	b, err := svc.CreateVMTA(ctx, &adminv1.CreateVMTARequest{Name: "edit-b", IpAddress: "203.0.113.62", EhloName: "edit-b.example.com", ListenerId: lb})
	if err != nil {
		t.Fatalf("create vmta b: %v", err)
	}
	g, err := svc.CreateVMTAGroups(ctx, &adminv1.CreateVMTAGroupRequest{
		Name: "edit-pool", Members: []*adminv1.VMTAGroupMember{{VmtaId: b.GetId(), Weight: 100}},
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	g2, err := svc.UpdateVMTAGroup(ctx, &adminv1.UpdateVMTAGroupRequest{
		Id: g.GetId(), Name: "edit-pool", Status: "active",
		Members: []*adminv1.VMTAGroupMember{{VmtaId: a.GetId(), Weight: 60}, {VmtaId: b.GetId(), Weight: 40}},
	})
	if err != nil {
		t.Fatalf("update group: %v", err)
	}
	if len(g2.GetMembers()) != 2 {
		t.Fatalf("group membership edit did not apply: %+v", g2.GetMembers())
	}

	// Editing a route to point at the group.
	r, err := svc.CreateRoutingRule(ctx, &adminv1.CreateRoutingRuleRequest{
		Name: "edit-route", MatchType: "mailclass", MatchValue: "bulk", Priority: 100,
		TargetType: "vmta", TargetId: b.GetId(),
	})
	if err != nil {
		t.Fatalf("create route: %v", err)
	}
	r2, err := svc.UpdateRoutingRule(ctx, &adminv1.UpdateRoutingRuleRequest{
		Id: r.GetId(), Name: "edit-route", MatchType: "mailclass", MatchValue: "bulk",
		Priority: 50, TargetType: "vmta_group", TargetId: g.GetId(), Status: "disabled",
	})
	if err != nil {
		t.Fatalf("update route: %v", err)
	}
	if r2.GetPriority() != 50 || r2.GetTargetType() != "vmta_group" || r2.GetStatus() != "disabled" {
		t.Fatalf("route edit did not apply: %+v", r2)
	}

	// Editing a non-existent VMTA is a not-found error.
	if _, err := svc.UpdateVMTA(ctx, &adminv1.UpdateVMTARequest{
		Id: "00000000-0000-0000-0000-0000000000ff", Name: "x", ListenerId: la,
	}); err == nil {
		t.Fatal("expected not-found editing a missing VMTA")
	}
}

// TestEditDKIMActivatesSigner verifies that editing a DKIM domain's status to
// "ready" makes it a signer in the generated policy — the activation path.
func TestEditDKIMActivatesSigner(t *testing.T) {
	svc := newService(t)
	ctx := ownerCtx()

	keyPEM, err := biz.GenerateDKIMPrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	d, err := svc.CreateDKIMDomain(ctx, &adminv1.CreateDKIMDomainRequest{
		Domain: "sign.example", Selector: "s1", PrivateKeyRef: keyPEM,
	})
	if err != nil {
		t.Fatalf("create dkim: %v", err)
	}
	if d.GetStatus() != "needs_attention" {
		t.Fatalf("expected needs_attention, got %q", d.GetStatus())
	}

	// Not a signer yet.
	before, _ := svc.GenerateKumoConfig(ctx, &adminv1.GenerateKumoConfigRequest{})
	if strings.Contains(before.GetContent(), `DKIM_BY_DOMAIN["sign.example"]`) {
		t.Fatal("domain should not sign before activation")
	}

	// Activate via edit.
	upd, err := svc.UpdateDKIMDomain(ctx, &adminv1.UpdateDKIMDomainRequest{
		Id: d.GetId(), Selector: "s1", Status: "ready",
	})
	if err != nil {
		t.Fatalf("update dkim: %v", err)
	}
	if upd.GetStatus() != "ready" {
		t.Fatalf("expected ready, got %q", upd.GetStatus())
	}

	// Now it signs.
	after, err := svc.GenerateKumoConfig(ctx, &adminv1.GenerateKumoConfigRequest{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !after.GetValid() || !strings.Contains(after.GetContent(), `DKIM_BY_DOMAIN["sign.example"]`) {
		t.Fatalf("activated DKIM domain should sign:\n%s", after.GetContent())
	}
}

// TestEditAuthorization verifies edits require the write permission.
func TestEditAuthorization(t *testing.T) {
	svc := newService(t)
	if _, err := svc.UpdateVMTA(t.Context(), &adminv1.UpdateVMTARequest{Id: "x"}); err == nil {
		t.Fatal("expected unauthenticated rejection")
	}
}
