package biz

import "context"

// DashboardSummary is the operator landing-page summary.
type DashboardSummary struct {
	ServiceState      string
	QueuedMessages    int64
	RecentMailEvents  int64
	RecentAuditEvents int64
}

// DashboardRepo is the persistence boundary for dashboard statistics.
type DashboardRepo interface {
	Summary(ctx context.Context) (*DashboardSummary, error)
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
