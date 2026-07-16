package biz

import (
	"context"
	"strings"
	"time"
)

// GlobalSettingsRepo is the persistence boundary for the singleton settings row.
type GlobalSettingsRepo interface {
	// Get returns the singleton row, seeding it on first access so callers
	// never see a not-found error.
	Get(ctx context.Context) (*GlobalSettings, error)
	// Update writes every field (the UI sends the complete state on save, so
	// clearing a field must round-trip), stamping the actor.
	Update(ctx context.Context, in *GlobalSettings, actor string) (*GlobalSettings, error)
}

// GlobalSettingsUsecase manages the deployment-level policy knobs that operators
// edit in the UI. The KumoMTA config generator reads the effective settings
// from here, so a change takes effect on the next Generate/Apply.
type GlobalSettingsUsecase struct {
	repo     GlobalSettingsRepo
	auditor  *Auditor
	fallback KumoConfigSettings
}

// NewGlobalSettingsUsecase constructs the use case. fallback supplies built-in
// defaults (from config/env) used when a stored field is empty.
func NewGlobalSettingsUsecase(repo GlobalSettingsRepo, auditor *Auditor, fallback KumoConfigSettings) *GlobalSettingsUsecase {
	return &GlobalSettingsUsecase{repo: repo, auditor: auditor, fallback: fallback}
}

// Get returns the current settings after a read authorization check.
func (uc *GlobalSettingsUsecase) Get(ctx context.Context) (*GlobalSettings, error) {
	if _, err := RequirePermission(ctx, PermSettingsRead); err != nil {
		return nil, err
	}
	return uc.repo.Get(ctx)
}

// Update validates and persists the settings, auditing the change.
func (uc *GlobalSettingsUsecase) Update(ctx context.Context, in *GlobalSettings) (*GlobalSettings, error) {
	id, err := RequirePermission(ctx, PermSettingsWrite)
	if err != nil {
		return nil, err
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.Update(ctx, in, id.UserID)
	if err != nil {
		uc.audit(ctx, AuditFailure, map[string]any{"rspamd_mode": in.RspamdMode})
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, map[string]any{
		"rspamd_mode": out.RspamdMode, "rspamd_url": out.RspamdURL,
		"egress_ehlo_domain": out.EgressEHLODomain,
		"esmtp_listen":       out.EsmtpListen, "http_listen": out.HTTPListen,
	})
	return out, nil
}

// Effective resolves the settings the config generator should use: stored
// values take precedence; empty stored fields fall back to config/env defaults.
// It performs no permission check — it is an internal provider for rendering.
func (uc *GlobalSettingsUsecase) Effective(ctx context.Context) (KumoConfigSettings, error) {
	row, err := uc.repo.Get(ctx)
	if err != nil {
		return uc.fallback, err
	}
	s := uc.fallback
	if row.RspamdMode != "" {
		s.RspamdMode = row.RspamdMode
	}
	if row.RspamdURL != "" {
		s.RspamdURL = row.RspamdURL
	}
	if row.EgressEHLODomain != "" {
		s.EgressEHLODefault = row.EgressEHLODomain
	}
	if row.LogStreamRedisURL != "" {
		s.LogStreamRedisURL = row.LogStreamRedisURL
	}
	if row.EsmtpListen != "" {
		s.EsmtpListen = row.EsmtpListen
	}
	if row.HTTPListen != "" {
		s.HTTPListen = row.HTTPListen
	}
	if row.EgressRetryInterval != "" {
		s.EgressRetryInterval = row.EgressRetryInterval
	}
	if row.EgressMaxRetryInterval != "" {
		s.EgressMaxRetryInterval = row.EgressMaxRetryInterval
	}
	if row.EgressMaxAge != "" {
		s.EgressMaxAge = row.EgressMaxAge
	}
	if row.BounceDomain != "" {
		s.BounceDomain = row.BounceDomain
	}
	if row.BounceDomainTemplate != "" {
		s.BounceDomainTemplate = row.BounceDomainTemplate
	}
	if row.DMARCReportEmail != "" {
		s.DMARCReportAddr = row.DMARCReportEmail
	}
	if row.InboundMaildirBasePath != "" {
		s.InboundMaildirBase = row.InboundMaildirBasePath
	}
	s.PinEgressPerMessage = row.PinEgressPerMessage
	return s, nil
}

// BouncePolicy is the runtime bounce-handling configuration consumed by the
// log-stream worker (auto-suppress hard bounces, soft-bounce threshold).
type BouncePolicy struct {
	AutoSuppressHardBounces bool
	SoftBounceThreshold     int
}

// FeedbackPolicy is the runtime FBL-handling configuration consumed by the
// log-stream worker.
type FeedbackPolicy struct {
	// RequireVerification suppresses a complainant only when the report was proven
	// to be about mail we sent.
	RequireVerification bool
}

// FeedbackPolicyNow returns the current FBL policy from the stored settings.
// Defaults to permissive (no verification required) on read failure.
func (uc *GlobalSettingsUsecase) FeedbackPolicyNow(ctx context.Context) FeedbackPolicy {
	row, err := uc.repo.Get(ctx)
	if err != nil || row == nil {
		return FeedbackPolicy{}
	}
	return FeedbackPolicy{RequireVerification: row.FBLRequireVerification}
}

// BouncePolicyNow returns the current bounce policy from the stored settings.
func (uc *GlobalSettingsUsecase) BouncePolicyNow(ctx context.Context) BouncePolicy {
	row, err := uc.repo.Get(ctx)
	if err != nil || row == nil {
		return BouncePolicy{AutoSuppressHardBounces: true}
	}
	return BouncePolicy{
		AutoSuppressHardBounces: row.AutoSuppressHardBounces,
		SoftBounceThreshold:     row.SoftBounceThreshold,
	}
}

// TLSAutoDisableNow reports whether the log processor may auto-disable TLS for a
// domain on a STARTTLS handshake failure. Internal provider (no permission gate);
// defaults false on any read error.
func (uc *GlobalSettingsUsecase) TLSAutoDisableNow(ctx context.Context) bool {
	row, err := uc.repo.Get(ctx)
	if err != nil || row == nil {
		return false
	}
	return row.TLSAutoDisable
}

// SuppressionTTLNow returns the configured suppression-record lifetime (0 when
// unset/permanent). Used by the suppression write path to set the Redis key TTL
// and the DB expires_at. No permission check — internal provider.
func (uc *GlobalSettingsUsecase) SuppressionTTLNow(ctx context.Context) time.Duration {
	row, err := uc.repo.Get(ctx)
	if err != nil || row == nil {
		return 0
	}
	d, _ := ParseFlexDuration(row.SuppressionTTL)
	return d
}

// RetryScheduleNow returns the effective outbound retry schedule (the configured
// egress retry interval / max interval / max age, falling back to KumoMTA's
// defaults when unset). No permission check — internal provider used to estimate
// a deferred message's next delivery attempt.
func (uc *GlobalSettingsUsecase) RetryScheduleNow(ctx context.Context) RetrySchedule {
	var s RetrySchedule
	row, err := uc.repo.Get(ctx)
	if err != nil || row == nil {
		return s
	}
	s.Interval, _ = ParseFlexDuration(row.EgressRetryInterval)
	s.MaxInterval, _ = ParseFlexDuration(row.EgressMaxRetryInterval)
	s.MaxAge, _ = ParseFlexDuration(row.EgressMaxAge)
	return s
}

// PrometheusURLNow returns the configured Prometheus base URL (empty when
// unset). Used by the metrics endpoint to decide whether time-series are
// available. No permission check — internal provider.
func (uc *GlobalSettingsUsecase) PrometheusURLNow(ctx context.Context) string {
	row, err := uc.repo.Get(ctx)
	if err != nil || row == nil {
		return ""
	}
	return strings.TrimSpace(row.PrometheusURL)
}

// ClassifyPolicyNow returns the current subject-classification policy (feature
// toggle + model/threshold/base). No permission check — internal provider read
// on each event by the log and classification workers so it hot-reloads.
func (uc *GlobalSettingsUsecase) ClassifyPolicyNow(ctx context.Context) ClassifyPolicy {
	row, err := uc.repo.Get(ctx)
	if err != nil || row == nil {
		return ClassifyPolicy{}
	}
	return ClassifyPolicy{
		Enabled:   row.ClassifySubjects,
		Model:     strings.TrimSpace(row.ClassifyModel),
		Threshold: row.ClassifyThreshold,
		APIBase:   strings.TrimSpace(row.ClassifyAPIBase),
	}
}

// MonitoringPolicyNow returns the current inbox-monitoring policy (fallback
// probe sender + pipeline tuning durations). Durations are 0 when unset — the
// consumer applies its built-in defaults. No permission check — internal
// provider read by the monitoring usecase/workers so changes hot-reload.
func (uc *GlobalSettingsUsecase) MonitoringPolicyNow(ctx context.Context) MonitoringPolicy {
	row, err := uc.repo.Get(ctx)
	if err != nil || row == nil {
		return MonitoringPolicy{}
	}
	p := MonitoringPolicy{From: strings.TrimSpace(row.MonitoringFrom)}
	p.ReconcileLookback, _ = ParseFlexDuration(row.MonitoringReconcileLookback)
	p.FetchTimeout, _ = ParseFlexDuration(row.MonitoringFetchTimeout)
	p.FetchGiveUp, _ = ParseFlexDuration(row.MonitoringFetchGiveUp)
	return p
}

func (uc *GlobalSettingsUsecase) audit(ctx context.Context, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, "settings.update", "global_settings", "1", outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", "settings.update", "error", err.Error())
	}
}
