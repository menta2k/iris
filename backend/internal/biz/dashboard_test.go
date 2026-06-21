package biz

import (
	"context"
	"testing"
)

type fakeDashboardRepo struct{ summary *DashboardSummary }

func (f fakeDashboardRepo) Summary(context.Context) (*DashboardSummary, error) {
	return f.summary, nil
}

func TestDashboardSummaryRequiresPermission(t *testing.T) {
	uc := NewDashboardUsecase(fakeDashboardRepo{summary: &DashboardSummary{}})
	ctx := WithIdentity(context.Background(), &Identity{
		Permissions: NewPermissionSet(nil), MFAVerified: true,
	})
	if _, err := uc.Summary(ctx); err == nil {
		t.Fatal("expected permission denied without dashboard:read")
	}
}

func TestDashboardSummaryReturnsData(t *testing.T) {
	want := &DashboardSummary{ServiceState: "running", QueuedMessages: 42, RecentMailEvents: 7, RecentAuditEvents: 3}
	uc := NewDashboardUsecase(fakeDashboardRepo{summary: want})
	got, err := uc.Summary(ownerCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.QueuedMessages != 42 || got.ServiceState != "running" {
		t.Fatalf("unexpected summary: %+v", got)
	}
}
