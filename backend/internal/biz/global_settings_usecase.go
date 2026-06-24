package biz

import (
	"context"
	"strings"
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
	return s, nil
}

// BouncePolicy is the runtime bounce-handling configuration consumed by the
// log-stream worker (auto-suppress hard bounces, soft-bounce threshold).
type BouncePolicy struct {
	AutoSuppressHardBounces bool
	SoftBounceThreshold     int
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

func (uc *GlobalSettingsUsecase) audit(ctx context.Context, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, "settings.update", "global_settings", "1", outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", "settings.update", "error", err.Error())
	}
}
