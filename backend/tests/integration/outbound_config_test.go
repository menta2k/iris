package integration

import (
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestOutboundConfigPersistence exercises the full VMTA -> group -> routing flow
// against TimescaleDB, verifying weighted membership and route persistence.
func TestOutboundConfigPersistence(t *testing.T) {
	db := setupDB(t)
	repo := data.NewOutboundConfigRepo(db)
	uc := biz.NewOutboundConfigUsecase(repo, nil)
	ctx := ownerCtx()

	la := seedListenerUC(t, uc, "lst-a", "203.0.113.20")
	lb := seedListenerUC(t, uc, "lst-b", "203.0.113.21")
	a, err := uc.CreateVMTA(ctx, &biz.VMTA{Name: "vmta-a", IPAddress: "203.0.113.20", EHLOName: "a.example.com", ListenerID: la})
	if err != nil {
		t.Fatalf("create vmta a: %v", err)
	}
	b, err := uc.CreateVMTA(ctx, &biz.VMTA{Name: "vmta-b", IPAddress: "203.0.113.21", EHLOName: "b.example.com", ListenerID: lb})
	if err != nil {
		t.Fatalf("create vmta b: %v", err)
	}

	group, err := uc.CreateVMTAGroup(ctx, &biz.VMTAGroup{
		Name: "pool", Members: []biz.VMTAGroupMember{
			{VMTAID: a.ID, Weight: 70}, {VMTAID: b.ID, Weight: 30},
		},
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if group.TotalWeight() != 100 {
		t.Fatalf("expected total weight 100, got %d", group.TotalWeight())
	}

	if _, err := uc.CreateRoutingRule(ctx, &biz.RoutingRule{
		Name: "bulk", MatchType: biz.MatchMailclass, MatchValue: "bulk",
		Priority: 100, TargetType: biz.TargetVMTAGroup, TargetID: group.ID,
	}); err != nil {
		t.Fatalf("create routing rule: %v", err)
	}

	rules, err := uc.ListRoutingRules(ctx, "", "", biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list routing rules: %v", err)
	}
	if len(rules) != 1 || rules[0].TargetID != group.ID {
		t.Fatalf("expected one route targeting the group, got %+v", rules)
	}

	groups, err := uc.ListVMTAGroups(ctx, biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 || len(groups[0].Members) != 2 {
		t.Fatalf("expected one group with two members, got %+v", groups)
	}
}

// TestSenderIPRoutingRule verifies a sender_ip classification rule round-trips
// through the repo: it persists with a null target and an assigned mailclass,
// and lists back intact.
func TestSenderIPRoutingRule(t *testing.T) {
	db := setupDB(t)
	uc := biz.NewOutboundConfigUsecase(data.NewOutboundConfigRepo(db), nil)
	ctx := ownerCtx()

	created, err := uc.CreateRoutingRule(ctx, &biz.RoutingRule{
		Name: "lab-subnet", MatchType: biz.MatchSenderIP, MatchValue: "10.1.111.0/24",
		AssignMailclass: "test-class", Priority: 200,
	})
	if err != nil {
		t.Fatalf("create sender_ip rule: %v", err)
	}
	if created.TargetID != "" || created.TargetType != "" {
		t.Fatalf("sender_ip rule should have no target, got type=%q id=%q", created.TargetType, created.TargetID)
	}
	if created.AssignMailclass != "test-class" {
		t.Fatalf("expected assign_mailclass test-class, got %q", created.AssignMailclass)
	}

	rules, err := uc.ListRoutingRules(ctx, biz.MatchSenderIP, "", biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 1 || rules[0].MatchValue != "10.1.111.0/24" || rules[0].AssignMailclass != "test-class" {
		t.Fatalf("unexpected sender_ip rules: %+v", rules)
	}

	// A sender_ip rule with no assigned class is rejected.
	if _, err := uc.CreateRoutingRule(ctx, &biz.RoutingRule{
		Name: "bad", MatchType: biz.MatchSenderIP, MatchValue: "10.0.0.1",
	}); err == nil {
		t.Fatal("expected rejection of sender_ip rule with no assign_mailclass")
	}
	// An invalid IP/CIDR is rejected.
	if _, err := uc.CreateRoutingRule(ctx, &biz.RoutingRule{
		Name: "bad2", MatchType: biz.MatchSenderIP, MatchValue: "not-an-ip", AssignMailclass: "x",
	}); err == nil {
		t.Fatal("expected rejection of invalid sender_ip match value")
	}
}

// TestUniqueActiveListenerBindRejected verifies the active listener bind
// (ip:port) uniqueness constraint maps to a conflict error.
func TestUniqueActiveListenerBindRejected(t *testing.T) {
	db := setupDB(t)
	uc := biz.NewOutboundConfigUsecase(data.NewOutboundConfigRepo(db), nil)
	ctx := ownerCtx()

	if _, err := uc.CreateListener(ctx, &biz.Listener{Name: "l1", IPAddress: "203.0.113.30", Port: 25, Hostname: "a.example.com"}); err != nil {
		t.Fatalf("create l1: %v", err)
	}
	_, err := uc.CreateListener(ctx, &biz.Listener{Name: "l2", IPAddress: "203.0.113.30", Port: 25, Hostname: "b.example.com"})
	if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindConflict {
		t.Fatalf("expected conflict for duplicate active listener bind, got %v", err)
	}
}

func seedListenerUC(t *testing.T, uc *biz.OutboundConfigUsecase, name, ip string) string {
	t.Helper()
	l, err := uc.CreateListener(ownerCtx(), &biz.Listener{Name: name, IPAddress: ip, Port: 25, Hostname: "mta.example.com"})
	if err != nil {
		t.Fatalf("seed listener: %v", err)
	}
	return l.ID
}
