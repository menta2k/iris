package biz

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// HostSampler reads the current host CPU / memory / disk usage. CPU is a delta
// measure, so implementations are stateful across calls.
type HostSampler interface {
	Sample(ctx context.Context, diskPaths []string) (SystemSnapshot, error)
	// Mounts lists the host's real filesystems so the operator can pick which to
	// monitor (the KumoMTA spool may live on a separate mount from "/").
	Mounts(ctx context.Context) ([]Mount, error)
}

// MonitorRepo persists the singleton monitor settings and the alert history.
type MonitorRepo interface {
	GetMonitorSettings(ctx context.Context) (*MonitorSettings, error)
	UpdateMonitorSettings(ctx context.Context, s *MonitorSettings) (*MonitorSettings, error)
	InsertMonitorAlert(ctx context.Context, a *MonitorAlert) error
	RecentMonitorAlerts(ctx context.Context, limit int) ([]*MonitorAlert, error)
}

// AlertNotifier delivers an alert email. Implementations are safe for concurrent
// use.
type AlertNotifier interface {
	Notify(ctx context.Context, host string, from string, to []string, subject, body string) error
}

// SysMonUsecase serves the current snapshot + settings + alert history to the
// API, and holds the latest snapshot the worker publishes.
type SysMonUsecase struct {
	repo     MonitorRepo
	notifier AlertNotifier
	sampler  HostSampler
	auditor  *Auditor

	mu   sync.RWMutex
	snap SystemSnapshot
}

// NewSysMonUsecase constructs the use case. notifier/sampler may be nil (Test
// and Mounts then report unavailable).
func NewSysMonUsecase(repo MonitorRepo, notifier AlertNotifier, sampler HostSampler, auditor *Auditor) *SysMonUsecase {
	return &SysMonUsecase{repo: repo, notifier: notifier, sampler: sampler, auditor: auditor}
}

// Mounts lists the host's real filesystems so the operator can choose which
// disks to monitor (e.g. the KumoMTA spool if it's on its own mount).
func (uc *SysMonUsecase) Mounts(ctx context.Context) ([]Mount, error) {
	if _, err := RequirePermission(ctx, PermSettingsRead); err != nil {
		return nil, err
	}
	if uc.sampler == nil {
		return nil, nil
	}
	return uc.sampler.Mounts(ctx)
}

// SetSnapshot publishes the worker's latest sample. Not permission-checked: it
// is an internal, worker-only call.
func (uc *SysMonUsecase) SetSnapshot(s SystemSnapshot) {
	uc.mu.Lock()
	uc.snap = s
	uc.mu.Unlock()
}

// Snapshot returns the most recent host sample (Available=false before the
// first one).
func (uc *SysMonUsecase) Snapshot(ctx context.Context) (SystemSnapshot, error) {
	if _, err := RequirePermission(ctx, PermSettingsRead); err != nil {
		return SystemSnapshot{}, err
	}
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	return uc.snap, nil
}

// Settings returns the current monitor settings.
func (uc *SysMonUsecase) Settings(ctx context.Context) (*MonitorSettings, error) {
	if _, err := RequirePermission(ctx, PermSettingsRead); err != nil {
		return nil, err
	}
	return uc.repo.GetMonitorSettings(ctx)
}

// UpdateSettings validates and persists the monitor settings.
func (uc *SysMonUsecase) UpdateSettings(ctx context.Context, s *MonitorSettings) (*MonitorSettings, error) {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return nil, err
	}
	if err := NormalizeMonitorSettings(s); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateMonitorSettings(ctx, s)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, "monitor.settings.update", map[string]any{"enabled": out.Enabled})
	return out, nil
}

// RecentAlerts returns the newest alert transitions.
func (uc *SysMonUsecase) RecentAlerts(ctx context.Context, limit int) ([]*MonitorAlert, error) {
	if _, err := RequirePermission(ctx, PermSettingsRead); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return uc.repo.RecentMonitorAlerts(ctx, limit)
}

// TestNotification sends a test alert email using the stored (or supplied)
// delivery settings, so the operator can verify the pipeline before relying on
// it. Returns the delivery error (nil = delivered).
func (uc *SysMonUsecase) TestNotification(ctx context.Context, s *MonitorSettings) error {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return err
	}
	if uc.notifier == nil {
		return Unavailable("MONITOR_NOTIFIER_UNAVAILABLE", "no notifier configured")
	}
	if err := NormalizeMonitorSettings(s); err != nil {
		return err
	}
	if len(s.NotifyEmails) == 0 || s.FromEmail == "" {
		return Invalid("MONITOR_TEST_INCOMPLETE", "from and notification email are required to send a test")
	}
	body := fmt.Sprintf("This is a test alert from iris self-monitoring, sent at %s.\n\n"+
		"If you received this, threshold alerts for CPU, memory, and disk will reach this address.",
		time.Now().UTC().Format(time.RFC3339))
	if err := uc.notifier.Notify(ctx, s.SMTPHost, s.FromEmail, s.NotifyEmails, "[iris] monitoring test alert", body); err != nil {
		return Unavailable("MONITOR_TEST_DELIVERY_FAILED", "test delivery failed: %v", err)
	}
	return nil
}

func (uc *SysMonUsecase) audit(ctx context.Context, action string, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, action, "monitor", "settings", AuditSuccess, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", action, "error", err.Error())
	}
}
