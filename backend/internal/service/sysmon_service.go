package service

import (
	"context"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// GetSystemMonitor returns the current host snapshot, monitor settings, and
// recent alert history.
func (s *Service) GetSystemMonitor(ctx context.Context, _ *adminv1.GetSystemMonitorRequest) (*adminv1.SystemMonitor, error) {
	if s.sysMon == nil {
		return nil, notImplemented("GetSystemMonitor")
	}
	snap, err := s.sysMon.Snapshot(ctx)
	if err != nil {
		return nil, s.fail(ctx, "GetSystemMonitor", err)
	}
	settings, err := s.sysMon.Settings(ctx)
	if err != nil {
		return nil, s.fail(ctx, "GetSystemMonitor", err)
	}
	alerts, err := s.sysMon.RecentAlerts(ctx, 50)
	if err != nil {
		return nil, s.fail(ctx, "GetSystemMonitor", err)
	}
	mounts, err := s.sysMon.Mounts(ctx)
	if err != nil {
		return nil, s.fail(ctx, "GetSystemMonitor", err)
	}
	out := &adminv1.SystemMonitor{
		Snapshot:  snapshotToProto(snap),
		Settings:  monitorSettingsToProto(settings),
		SpoolPath: biz.KumoSpoolPath,
	}
	for _, a := range alerts {
		out.RecentAlerts = append(out.RecentAlerts, monitorAlertToProto(a))
	}
	for _, m := range mounts {
		out.Mounts = append(out.Mounts, &adminv1.Mount{
			Path: m.Path, Device: m.Device, Fstype: m.FSType,
			UsedPercent: m.UsedPercent, UsedBytes: m.UsedBytes, TotalBytes: m.TotalBytes,
		})
	}
	return out, nil
}

// GetSystemMetrics returns host CPU / memory / per-disk usage over time from the
// iris_system_* Prometheus gauges.
func (s *Service) GetSystemMetrics(ctx context.Context, req *adminv1.GetSystemMetricsRequest) (*adminv1.MetricsTimeseries, error) {
	if s.metrics == nil {
		return nil, notImplemented("GetSystemMetrics")
	}
	ts, err := s.metrics.SystemTimeseries(ctx, req.GetRange())
	if err != nil {
		return nil, s.fail(ctx, "GetSystemMetrics", err)
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

// UpdateMonitorSettings validates and persists the monitor settings.
func (s *Service) UpdateMonitorSettings(ctx context.Context, req *adminv1.UpdateMonitorSettingsRequest) (*adminv1.MonitorSettings, error) {
	if s.sysMon == nil {
		return nil, notImplemented("UpdateMonitorSettings")
	}
	out, err := s.sysMon.UpdateSettings(ctx, monitorSettingsFromProto(req.GetSettings()))
	if err != nil {
		return nil, s.fail(ctx, "UpdateMonitorSettings", err)
	}
	return monitorSettingsToProto(out), nil
}

// TestMonitorNotification sends a test alert email with the supplied settings. A
// delivery failure is a normal reportable outcome, not an RPC error.
func (s *Service) TestMonitorNotification(ctx context.Context, req *adminv1.TestMonitorNotificationRequest) (*adminv1.TestMonitorNotificationReply, error) {
	if s.sysMon == nil {
		return nil, notImplemented("TestMonitorNotification")
	}
	if err := s.sysMon.TestNotification(ctx, monitorSettingsFromProto(req.GetSettings())); err != nil {
		if de, ok := err.(*biz.DomainError); ok && de.Reason == "MONITOR_TEST_DELIVERY_FAILED" {
			return &adminv1.TestMonitorNotificationReply{Ok: false, Error: de.Message}, nil
		}
		return nil, s.fail(ctx, "TestMonitorNotification", err)
	}
	return &adminv1.TestMonitorNotificationReply{Ok: true}, nil
}

func monitorSettingsToProto(s *biz.MonitorSettings) *adminv1.MonitorSettings {
	if s == nil {
		return &adminv1.MonitorSettings{}
	}
	return &adminv1.MonitorSettings{
		Enabled:         s.Enabled,
		CpuThreshold:    int32(s.CPUThreshold),
		MemThreshold:    int32(s.MemThreshold),
		DiskThreshold:   int32(s.DiskThreshold),
		DiskPaths:       s.DiskPaths,
		NotifyEmails:    s.NotifyEmails,
		FromEmail:       s.FromEmail,
		SmtpHost:        s.SMTPHost,
		CooldownMinutes: int32(s.CooldownMinutes),
		SampleSeconds:   int32(s.SampleSeconds),
	}
}

func monitorSettingsFromProto(p *adminv1.MonitorSettings) *biz.MonitorSettings {
	if p == nil {
		return &biz.MonitorSettings{}
	}
	return &biz.MonitorSettings{
		Enabled:         p.GetEnabled(),
		CPUThreshold:    int(p.GetCpuThreshold()),
		MemThreshold:    int(p.GetMemThreshold()),
		DiskThreshold:   int(p.GetDiskThreshold()),
		DiskPaths:       p.GetDiskPaths(),
		NotifyEmails:    p.GetNotifyEmails(),
		FromEmail:       p.GetFromEmail(),
		SMTPHost:        p.GetSmtpHost(),
		CooldownMinutes: int(p.GetCooldownMinutes()),
		SampleSeconds:   int(p.GetSampleSeconds()),
	}
}

func snapshotToProto(s biz.SystemSnapshot) *adminv1.SystemSnapshot {
	out := &adminv1.SystemSnapshot{
		CpuPercent:    s.CPUPercent,
		MemPercent:    s.MemPercent,
		MemUsedBytes:  s.MemUsedBytes,
		MemTotalBytes: s.MemTotalBytes,
		Available:     s.Available,
	}
	if !s.CollectedAt.IsZero() {
		out.CollectedAt = s.CollectedAt.UTC().Format(time.RFC3339)
	}
	for _, d := range s.Disks {
		out.Disks = append(out.Disks, &adminv1.DiskUsage{
			Path: d.Path, UsedPercent: d.UsedPercent, UsedBytes: d.UsedBytes, TotalBytes: d.TotalBytes,
		})
	}
	return out
}

func monitorAlertToProto(a *biz.MonitorAlert) *adminv1.MonitorAlert {
	return &adminv1.MonitorAlert{
		Id: a.ID, Resource: a.Resource, Detail: a.Detail, Level: a.Level, Value: a.Value,
		Threshold: int32(a.Threshold), Message: a.Message, Notified: a.Notified,
		CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
	}
}
