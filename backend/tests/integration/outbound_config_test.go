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
	a, err := uc.CreateVMTA(ctx, &biz.VMTA{Name: "vmta-a", ListenerID: la})
	if err != nil {
		t.Fatalf("create vmta a: %v", err)
	}
	b, err := uc.CreateVMTA(ctx, &biz.VMTA{Name: "vmta-b", ListenerID: lb})
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
