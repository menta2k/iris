package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
)

// GetDashboardSummary returns the operator dashboard summary (US6).
func (s *Service) GetDashboardSummary(ctx context.Context, req *adminv1.GetDashboardSummaryRequest) (*adminv1.DashboardSummary, error) {
	if s.dashboard == nil {
		return nil, notImplemented("GetDashboardSummary")
	}
	summary, err := s.dashboard.Summary(ctx)
	if err != nil {
		return nil, s.fail(ctx, "GetDashboardSummary", err)
	}
	return &adminv1.DashboardSummary{
		ServiceState:      summary.ServiceState,
		QueuedMessages:    summary.QueuedMessages,
		RecentMailEvents:  summary.RecentMailEvents,
		RecentAuditEvents: summary.RecentAuditEvents,
	}, nil
}
