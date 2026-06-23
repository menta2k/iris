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

// GetMetricsTimeseries returns curated mail-flow time-series from the configured
// Prometheus (deliveries/receptions/deferrals/bounces per minute).
func (s *Service) GetMetricsTimeseries(ctx context.Context, req *adminv1.GetMetricsTimeseriesRequest) (*adminv1.MetricsTimeseries, error) {
	if s.metrics == nil {
		return nil, notImplemented("GetMetricsTimeseries")
	}
	ts, err := s.metrics.Timeseries(ctx, req.GetRange())
	if err != nil {
		return nil, s.fail(ctx, "GetMetricsTimeseries", err)
	}
	out := &adminv1.MetricsTimeseries{
		Range:               ts.Range,
		StepSeconds:         ts.StepSeconds,
		PrometheusAvailable: ts.PrometheusAvailable,
	}
	for _, ser := range ts.Series {
		ps := &adminv1.MetricsSeries{Key: ser.Key, Label: ser.Label}
		for _, p := range ser.Points {
			ps.Points = append(ps.Points, &adminv1.MetricPoint{Timestamp: p.Timestamp, Value: p.Value})
		}
		out.Series = append(out.Series, ps)
	}
	return out, nil
}
