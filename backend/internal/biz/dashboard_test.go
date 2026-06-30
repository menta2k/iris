package biz

import (
	"context"
	"testing"
	"time"
)

type fakeDashboardRepo struct {
	summary *DashboardSummary
	stats   []WarmupDeliveryStat
	since   time.Time
}

func (f *fakeDashboardRepo) Summary(context.Context) (*DashboardSummary, error) {
	return f.summary, nil
}

func (f *fakeDashboardRepo) DeliveryStats(_ context.Context, since time.Time) ([]WarmupDeliveryStat, error) {
	f.since = since
	return f.stats, nil
}

func TestDashboardSummaryRequiresPermission(t *testing.T) {
	uc := NewDashboardUsecase(&fakeDashboardRepo{summary: &DashboardSummary{}})
	ctx := WithIdentity(context.Background(), &Identity{
		Permissions: NewPermissionSet(nil), MFAVerified: true,
	})
	if _, err := uc.Summary(ctx); err == nil {
		t.Fatal("expected permission denied without dashboard:read")
	}
}

func TestDashboardSummaryReturnsData(t *testing.T) {
	want := &DashboardSummary{ServiceState: "running", QueuedMessages: 42, RecentMailEvents: 7, RecentAuditEvents: 3}
	uc := NewDashboardUsecase(&fakeDashboardRepo{summary: want})
	got, err := uc.Summary(ownerCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.QueuedMessages != 42 || got.ServiceState != "running" {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestWarmupDeliveryStatsRequiresPermission(t *testing.T) {
	uc := NewDashboardUsecase(&fakeDashboardRepo{})
	ctx := WithIdentity(context.Background(), &Identity{
		Permissions: NewPermissionSet(nil), MFAVerified: true,
	})
	if _, err := uc.WarmupDeliveryStats(ctx, "24h"); err == nil {
		t.Fatal("expected permission denied without dashboard:read")
	}
}

func TestWarmupDeliveryStatsDerivesRates(t *testing.T) {
	repo := &fakeDashboardRepo{stats: []WarmupDeliveryStat{
		{VMTAName: "ip-a", RecipientDomain: "gmail.com", Sent: 90, Bounced: 10, Deferred: 5},
		{VMTAName: "ip-b", RecipientDomain: "yahoo.com", Sent: 0, Bounced: 0, Deferred: 3},
	}}
	uc := NewDashboardUsecase(repo)
	res, err := uc.WarmupDeliveryStats(ownerCtx(), "24h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Range != "24h" {
		t.Fatalf("range = %q, want 24h", res.Range)
	}
	a := res.Rows[0]
	if a.Attempted != 100 {
		t.Fatalf("attempted = %d, want 100", a.Attempted)
	}
	if a.DeliveryRate != 0.9 || a.BounceRate != 0.1 {
		t.Fatalf("rates = %.3f/%.3f, want 0.900/0.100", a.DeliveryRate, a.BounceRate)
	}
	// No terminal traffic must not divide by zero.
	b := res.Rows[1]
	if b.Attempted != 0 || b.DeliveryRate != 0 || b.BounceRate != 0 {
		t.Fatalf("zero-terminal row: %+v", b)
	}
}
