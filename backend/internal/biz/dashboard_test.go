package biz

import (
	"context"
	"testing"
	"time"
)

type fakeDashboardRepo struct {
	summary          *DashboardSummary
	stats            []WarmupDeliveryStat
	deferredByDomain []DomainDeferredStat
	classStats       []MailClassStat
	domainStats      []RecipientDomainStat
	domainLimit      int
	since            time.Time
}

func (f *fakeDashboardRepo) Summary(context.Context) (*DashboardSummary, error) {
	return f.summary, nil
}

func (f *fakeDashboardRepo) DeliveryStats(_ context.Context, since time.Time, _ string) ([]WarmupDeliveryStat, error) {
	f.since = since
	return f.stats, nil
}

func (f *fakeDashboardRepo) DeferredByDomain(_ context.Context, since time.Time, _ string) ([]DomainDeferredStat, error) {
	f.since = since
	return f.deferredByDomain, nil
}

func (f *fakeDashboardRepo) MailClassStats(_ context.Context, since time.Time, _ string) ([]MailClassStat, error) {
	f.since = since
	return f.classStats, nil
}

func (f *fakeDashboardRepo) RecipientDomainStats(_ context.Context, since time.Time, _ string, limit int) ([]RecipientDomainStat, error) {
	f.since = since
	f.domainLimit = limit
	return f.domainStats, nil
}

func TestDashboardSummaryUsesLiveQueueDepth(t *testing.T) {
	// Repo reports a stale/empty queued count; the live queue admin overrides it
	// with the sum of scheduled depths. (fakeQueueAdmin is in queue_action_test.go.)
	repo := &fakeDashboardRepo{summary: &DashboardSummary{QueuedMessages: 0, ServiceState: "running"}}
	queue := &fakeQueueAdmin{summary: []*QueueState{
		{Domain: "gmail.com", Depth: 374},
		{Domain: "abv.bg", Depth: 40},
		{Domain: "yahoo.com", Depth: 7},
	}}
	uc := NewDashboardUsecase(repo).WithQueueAdmin(queue)
	got, err := uc.Summary(ownerCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.QueuedMessages != 421 {
		t.Fatalf("queued = %d, want 421 (sum of live scheduled depths)", got.QueuedMessages)
	}
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
	if _, err := uc.WarmupDeliveryStats(ctx, "24h", ""); err == nil {
		t.Fatal("expected permission denied without dashboard:read")
	}
}

func TestWarmupDeliveryStatsDerivesRates(t *testing.T) {
	repo := &fakeDashboardRepo{stats: []WarmupDeliveryStat{
		{VMTAName: "ip-a", RecipientDomain: "gmail.com", Sent: 90, Bounced: 10, Deferred: 5},
		{VMTAName: "ip-b", RecipientDomain: "yahoo.com", Sent: 0, Bounced: 0, Deferred: 3},
	}}
	uc := NewDashboardUsecase(repo)
	res, err := uc.WarmupDeliveryStats(ownerCtx(), "24h", "")
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

func TestWarmupDeliveryStatsIncludesDeferredByDomain(t *testing.T) {
	repo := &fakeDashboardRepo{deferredByDomain: []DomainDeferredStat{
		{RecipientDomain: "gmail.com", Messages: 232},
		{RecipientDomain: "abv.bg", Messages: 12},
	}}
	uc := NewDashboardUsecase(repo)
	res, err := uc.WarmupDeliveryStats(ownerCtx(), "24h", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.DeferredByDomain) != 2 || res.DeferredByDomain[0].RecipientDomain != "gmail.com" ||
		res.DeferredByDomain[0].Messages != 232 {
		t.Fatalf("unexpected deferred-by-domain: %+v", res.DeferredByDomain)
	}
}

func TestMailClassStatsRequiresPermission(t *testing.T) {
	uc := NewDashboardUsecase(&fakeDashboardRepo{})
	ctx := WithIdentity(context.Background(), &Identity{
		Permissions: NewPermissionSet(nil), MFAVerified: true,
	})
	if _, err := uc.MailClassStats(ctx, "24h", ""); err == nil {
		t.Fatal("expected permission denied without dashboard:read")
	}
}

func TestMailClassStatsReturnsRows(t *testing.T) {
	repo := &fakeDashboardRepo{classStats: []MailClassStat{
		{Mailclass: "transactional", Count: 100, Delivered: 90, Bounced: 5, Deferred: 5},
	}}
	uc := NewDashboardUsecase(repo)
	res, err := uc.MailClassStats(ownerCtx(), "6h", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Range != "6h" || len(res.Rows) != 1 || res.Rows[0].Count != 100 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRecipientDomainStatsRequiresPermission(t *testing.T) {
	uc := NewDashboardUsecase(&fakeDashboardRepo{})
	ctx := WithIdentity(context.Background(), &Identity{
		Permissions: NewPermissionSet(nil), MFAVerified: true,
	})
	if _, err := uc.RecipientDomainStats(ctx, "24h", ""); err == nil {
		t.Fatal("expected permission denied without dashboard:read")
	}
}

func TestRecipientDomainStatsPassesTopTenLimit(t *testing.T) {
	repo := &fakeDashboardRepo{domainStats: []RecipientDomainStat{
		{RecipientDomain: "gmail.com", Count: 500, Delivered: 480, Bounced: 20},
	}}
	uc := NewDashboardUsecase(repo)
	res, err := uc.RecipientDomainStats(ownerCtx(), "7d", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.domainLimit != topRecipientDomains {
		t.Fatalf("limit = %d, want %d", repo.domainLimit, topRecipientDomains)
	}
	if res.Range != "7d" || len(res.Rows) != 1 || res.Rows[0].RecipientDomain != "gmail.com" {
		t.Fatalf("unexpected result: %+v", res)
	}
}
