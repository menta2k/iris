package biz

import (
	"context"
	"strings"
	"time"
)

// DashboardSummary is the operator landing-page summary.
type DashboardSummary struct {
	ServiceState      string
	QueuedMessages    int64
	RecentMailEvents  int64
	RecentAuditEvents int64
	// DeferredInQueue counts messages currently deferred in the queue (a
	// transient failure logged, no terminal delivery/bounce since).
	DeferredInQueue int64
}

// WarmupDeliveryStat is one (VMTA, recipient-domain) delivery/bounce breakdown
// over a lookback window, used to watch IP-warmup health. The repo fills the
// raw counts (Sent/Bounced/Deferred); the usecase derives Attempted and the
// two rates so the math stays testable without a database.
type WarmupDeliveryStat struct {
	VMTAID          string
	VMTAName        string
	RecipientDomain string
	Sent            int64
	Bounced         int64
	Deferred        int64
	Attempted       int64   // Sent + Bounced (terminal outcomes)
	DeliveryRate    float64 // Sent / Attempted, 0..1
	BounceRate      float64 // Bounced / Attempted, 0..1
}

// WarmupDeliveryStatsResult is the dashboard warmup panel payload.
type WarmupDeliveryStatsResult struct {
	Rows  []WarmupDeliveryStat
	Range string // echoed effective range
	Since int64  // unix seconds: window start
}

// MailClassStat is the mail-record volume for one mailclass over a window.
// Count is every record; Delivered/Bounced/Deferred break it down by outcome
// (delivered maps to the "sent" delivery status).
type MailClassStat struct {
	Mailclass string
	Count     int64
	Delivered int64
	Bounced   int64
	Deferred  int64
}

// MailClassStatsResult is the dashboard "mail by class" panel payload.
type MailClassStatsResult struct {
	Rows  []MailClassStat
	Range string // echoed effective range
	Since int64  // unix seconds: window start
}

// RecipientDomainStat is the mail-record volume for one recipient domain over a
// window (the busiest domains, ranked by Count).
type RecipientDomainStat struct {
	RecipientDomain string
	Count           int64
	Delivered       int64
	Bounced         int64
	Deferred        int64
}

// RecipientDomainStatsResult is the dashboard "top recipient domains" payload.
type RecipientDomainStatsResult struct {
	Rows  []RecipientDomainStat
	Range string // echoed effective range
	Since int64  // unix seconds: window start
}

// DashboardRepo is the persistence boundary for dashboard statistics.
type DashboardRepo interface {
	Summary(ctx context.Context) (*DashboardSummary, error)
	// DeliveryStats returns per-VMTA, per-recipient-domain raw counts for events
	// at or after since. Rate fields are left zero for the usecase to derive.
	DeliveryStats(ctx context.Context, since time.Time) ([]WarmupDeliveryStat, error)
	// MailClassStats returns mail-record counts grouped by mailclass for events
	// at or after since, ordered by total descending.
	MailClassStats(ctx context.Context, since time.Time) ([]MailClassStat, error)
	// RecipientDomainStats returns mail-record counts grouped by recipient domain
	// for events at or after since, ordered by total descending, capped at limit.
	RecipientDomainStats(ctx context.Context, since time.Time, limit int) ([]RecipientDomainStat, error)
}

// DashboardUsecase implements the dashboard summary (US6).
type DashboardUsecase struct {
	repo DashboardRepo
}

// NewDashboardUsecase constructs the use case.
func NewDashboardUsecase(repo DashboardRepo) *DashboardUsecase {
	return &DashboardUsecase{repo: repo}
}

// Summary returns the dashboard summary after an authorization check.
func (uc *DashboardUsecase) Summary(ctx context.Context) (*DashboardSummary, error) {
	if _, err := RequirePermission(ctx, PermDashboardRead); err != nil {
		return nil, err
	}
	return uc.repo.Summary(ctx)
}

// warmupStatsLookback maps a range token to its lookback duration and the
// effective range echoed back. Unknown ranges fall back to "24h".
func warmupStatsLookback(r string) (time.Duration, string) {
	switch strings.TrimSpace(r) {
	case "1h":
		return time.Hour, "1h"
	case "6h":
		return 6 * time.Hour, "6h"
	case "7d":
		return 7 * 24 * time.Hour, "7d"
	case "24h", "":
		return 24 * time.Hour, "24h"
	default:
		return 24 * time.Hour, "24h"
	}
}

// WarmupDeliveryStats returns per-VMTA, per-recipient-domain delivery and bounce
// rates over the given lookback window. Rates are computed over terminal
// outcomes (Sent + Bounced) so DeliveryRate + BounceRate == 1 when there is any
// terminal traffic; deferrals are reported separately and excluded from the
// denominator to avoid double-counting a message that defers then delivers.
func (uc *DashboardUsecase) WarmupDeliveryStats(ctx context.Context, rng string) (*WarmupDeliveryStatsResult, error) {
	if _, err := RequirePermission(ctx, PermDashboardRead); err != nil {
		return nil, err
	}
	lookback, eff := warmupStatsLookback(rng)
	since := time.Now().Add(-lookback)

	rows, err := uc.repo.DeliveryStats(ctx, since)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Attempted = rows[i].Sent + rows[i].Bounced
		if rows[i].Attempted > 0 {
			rows[i].DeliveryRate = float64(rows[i].Sent) / float64(rows[i].Attempted)
			rows[i].BounceRate = float64(rows[i].Bounced) / float64(rows[i].Attempted)
		}
	}
	return &WarmupDeliveryStatsResult{Rows: rows, Range: eff, Since: since.Unix()}, nil
}

// topRecipientDomains caps the "top recipient domains" panel.
const topRecipientDomains = 10

// MailClassStats returns mail-record volume grouped by mailclass over the given
// lookback window, ordered by total descending.
func (uc *DashboardUsecase) MailClassStats(ctx context.Context, rng string) (*MailClassStatsResult, error) {
	if _, err := RequirePermission(ctx, PermDashboardRead); err != nil {
		return nil, err
	}
	lookback, eff := warmupStatsLookback(rng)
	since := time.Now().Add(-lookback)

	rows, err := uc.repo.MailClassStats(ctx, since)
	if err != nil {
		return nil, err
	}
	return &MailClassStatsResult{Rows: rows, Range: eff, Since: since.Unix()}, nil
}

// RecipientDomainStats returns the busiest recipient domains by mail-record
// volume over the given lookback window (top 10).
func (uc *DashboardUsecase) RecipientDomainStats(ctx context.Context, rng string) (*RecipientDomainStatsResult, error) {
	if _, err := RequirePermission(ctx, PermDashboardRead); err != nil {
		return nil, err
	}
	lookback, eff := warmupStatsLookback(rng)
	since := time.Now().Add(-lookback)

	rows, err := uc.repo.RecipientDomainStats(ctx, since, topRecipientDomains)
	if err != nil {
		return nil, err
	}
	return &RecipientDomainStatsResult{Rows: rows, Range: eff, Since: since.Unix()}, nil
}
