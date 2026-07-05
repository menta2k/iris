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
		DeferredInQueue:   summary.DeferredInQueue,
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

// GetQueueTimeHistogram returns the delivery queue-time distribution over the
// selected window, global or narrowed to one mail class.
func (s *Service) GetQueueTimeHistogram(ctx context.Context, req *adminv1.GetQueueTimeHistogramRequest) (*adminv1.QueueTimeHistogram, error) {
	if s.metrics == nil {
		return nil, notImplemented("GetQueueTimeHistogram")
	}
	h, err := s.metrics.QueueTimeHistogram(ctx, req.GetRange(), req.GetMailclass())
	if err != nil {
		return nil, s.fail(ctx, "GetQueueTimeHistogram", err)
	}
	out := &adminv1.QueueTimeHistogram{
		Mailclasses:         h.Mailclasses,
		TotalCount:          h.TotalCount,
		Range:               h.Range,
		PrometheusAvailable: h.PrometheusAvailable,
	}
	for _, b := range h.Buckets {
		out.Buckets = append(out.Buckets, &adminv1.QueueTimeBucket{
			Le: b.Le, UpperBound: b.UpperBound, Count: b.Count,
		})
	}
	return out, nil
}

// GetWarmupDeliveryStats returns per-VMTA, per-recipient-domain delivery and
// bounce rates over the selected window (IP-warmup health).
func (s *Service) GetWarmupDeliveryStats(ctx context.Context, req *adminv1.GetWarmupDeliveryStatsRequest) (*adminv1.WarmupDeliveryStats, error) {
	if s.dashboard == nil {
		return nil, notImplemented("GetWarmupDeliveryStats")
	}
	res, err := s.dashboard.WarmupDeliveryStats(ctx, req.GetRange())
	if err != nil {
		return nil, s.fail(ctx, "GetWarmupDeliveryStats", err)
	}
	out := &adminv1.WarmupDeliveryStats{Range: res.Range, Since: res.Since}
	for _, row := range res.Rows {
		out.Rows = append(out.Rows, &adminv1.WarmupDeliveryStat{
			VmtaId:          row.VMTAID,
			VmtaName:        row.VMTAName,
			RecipientDomain: row.RecipientDomain,
			Sent:            row.Sent,
			Bounced:         row.Bounced,
			Deferred:        row.Deferred,
			Attempted:       row.Attempted,
			DeliveryRate:    row.DeliveryRate,
			BounceRate:      row.BounceRate,
		})
	}
	for _, d := range res.DeferredByDomain {
		out.DeferredByDomain = append(out.DeferredByDomain, &adminv1.DomainDeferredStat{
			RecipientDomain: d.RecipientDomain,
			Messages:        d.Messages,
		})
	}
	return out, nil
}

// GetMailClassStats returns mail-record volume grouped by mailclass over the
// selected window ("mail by class" dashboard panel).
func (s *Service) GetMailClassStats(ctx context.Context, req *adminv1.GetMailClassStatsRequest) (*adminv1.MailClassStats, error) {
	if s.dashboard == nil {
		return nil, notImplemented("GetMailClassStats")
	}
	res, err := s.dashboard.MailClassStats(ctx, req.GetRange())
	if err != nil {
		return nil, s.fail(ctx, "GetMailClassStats", err)
	}
	out := &adminv1.MailClassStats{Range: res.Range, Since: res.Since}
	for _, row := range res.Rows {
		out.Rows = append(out.Rows, &adminv1.MailClassStat{
			Mailclass: row.Mailclass,
			Count:     row.Count,
			Delivered: row.Delivered,
			Bounced:   row.Bounced,
			Deferred:  row.Deferred,
		})
	}
	return out, nil
}

// GetRecipientDomainStats returns the busiest recipient domains by mail-record
// volume over the selected window ("top recipient domains" dashboard panel).
func (s *Service) GetRecipientDomainStats(ctx context.Context, req *adminv1.GetRecipientDomainStatsRequest) (*adminv1.RecipientDomainStats, error) {
	if s.dashboard == nil {
		return nil, notImplemented("GetRecipientDomainStats")
	}
	res, err := s.dashboard.RecipientDomainStats(ctx, req.GetRange())
	if err != nil {
		return nil, s.fail(ctx, "GetRecipientDomainStats", err)
	}
	out := &adminv1.RecipientDomainStats{Range: res.Range, Since: res.Since}
	for _, row := range res.Rows {
		out.Rows = append(out.Rows, &adminv1.RecipientDomainStat{
			RecipientDomain: row.RecipientDomain,
			Count:           row.Count,
			Delivered:       row.Delivered,
			Bounced:         row.Bounced,
			Deferred:        row.Deferred,
		})
	}
	return out, nil
}
