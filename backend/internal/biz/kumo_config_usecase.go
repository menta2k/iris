package biz

import (
	"context"
	"strings"
	"time"
)

// ConfigSnapshotLoader loads the active configuration snapshot for rendering.
type ConfigSnapshotLoader interface {
	Snapshot(ctx context.Context) (ConfigSnapshot, error)
}

// KumoConfigSettings are deployment-level policy knobs that are not stored as
// per-entity configuration: inbound rspamd filtering, the log stream, listener
// binds, and the default egress EHLO. They are applied to every rendered
// snapshot.
type KumoConfigSettings struct {
	RspamdMode        string
	RspamdURL         string
	LogStreamRedisURL string
	LogStreamName     string
	EsmtpListen       string
	HTTPListen        string
	EgressEHLODefault string

	// Delivery rates (outbound retry schedule).
	EgressRetryInterval    string
	EgressMaxRetryInterval string
	EgressMaxAge           string

	// Bounce domain that inbound DSNs are accepted at (DSN pipeline).
	BounceDomain string

	// FBLDomains enables ARF parsing (log_arf) for the feedback pipeline, one
	// entry per ARF-candidate domain. Empty disables the pipeline.
	FBLDomains []string

	// BounceClassifierFile is the KumoMTA bounce-classifier rules file loaded in
	// the init block (empty disables classification).
	BounceClassifierFile string

	// BounceVerpSecret signs the VERP envelope return-path. Empty disables VERP
	// (the bounce pipeline still works, but async DSNs aren't correlated).
	BounceVerpSecret string
}

// KumoConfigUsecase renders Iris configuration into KumoMTA policy and applies
// it to the running service. Apply is high-risk: it requires the service
// control permission, an explicit confirmation, is serialized through the
// service-control request table, and is audited.
type KumoConfigUsecase struct {
	loader   ConfigSnapshotLoader
	kumo     KumoMTAAdapter
	scStore  ServiceControlStore
	auditor  *Auditor
	settings EffectiveSettingsProvider
}

// EffectiveSettingsProvider resolves the deployment-level policy settings the
// generator should use (UI-managed global settings merged over config/env
// defaults). The GlobalSettings use case implements it.
type EffectiveSettingsProvider interface {
	Effective(ctx context.Context) (KumoConfigSettings, error)
}

type staticSettings KumoConfigSettings

func (s staticSettings) Effective(context.Context) (KumoConfigSettings, error) {
	return KumoConfigSettings(s), nil
}

// StaticSettings wraps a fixed KumoConfigSettings as a provider (used in tests
// and when no settings store is wired).
func StaticSettings(s KumoConfigSettings) EffectiveSettingsProvider { return staticSettings(s) }

// ServiceControlStore is the subset of the mail-ops repository the config use
// case needs to serialize and record apply operations.
type ServiceControlStore interface {
	CreateServiceControlRequest(ctx context.Context, rec *ServiceControlRecord) (*ServiceControlRecord, error)
	ActiveServiceControlExists(ctx context.Context) (bool, error)
	UpdateServiceControlStatus(ctx context.Context, id, status, resultSummary string) error
	// GetAppliedChecksum returns the full + init-block checksums of the last
	// successfully applied policy (empty if never applied) and when it was applied.
	GetAppliedChecksum(ctx context.Context) (checksum, initChecksum string, appliedAt *time.Time, err error)
	// SetAppliedChecksum records a successful apply.
	SetAppliedChecksum(ctx context.Context, checksum, initChecksum, by string) error
}

// NewKumoConfigUsecase constructs the use case. settings supplies the effective
// deployment-level policy knobs; nil falls back to empty settings.
func NewKumoConfigUsecase(loader ConfigSnapshotLoader, kumo KumoMTAAdapter, scStore ServiceControlStore, auditor *Auditor, settings EffectiveSettingsProvider) *KumoConfigUsecase {
	if settings == nil {
		settings = staticSettings(KumoConfigSettings{})
	}
	return &KumoConfigUsecase{loader: loader, kumo: kumo, scStore: scStore, auditor: auditor, settings: settings}
}

// Generate renders the current configuration into KumoMTA policy without
// applying it. It requires read access to outbound configuration.
func (uc *KumoConfigUsecase) Generate(ctx context.Context) (RenderedConfig, error) {
	if _, err := RequirePermission(ctx, PermVMTARead); err != nil {
		return RenderedConfig{}, err
	}
	return uc.render(ctx)
}

func (uc *KumoConfigUsecase) render(ctx context.Context) (RenderedConfig, error) {
	snap, err := uc.loader.Snapshot(ctx)
	if err != nil {
		return RenderedConfig{}, err
	}
	if id := IdentityFrom(ctx); id != nil {
		snap.GeneratedBy = id.Email
	}
	settings, err := uc.settings.Effective(ctx)
	if err != nil {
		return RenderedConfig{}, err
	}
	snap.RspamdMode = settings.RspamdMode
	snap.RspamdURL = settings.RspamdURL
	snap.LogStreamRedisURL = settings.LogStreamRedisURL
	snap.LogStreamName = settings.LogStreamName
	snap.EsmtpListen = settings.EsmtpListen
	snap.HTTPListen = settings.HTTPListen
	snap.EgressEHLODefault = settings.EgressEHLODefault
	snap.EgressRetryInterval = settings.EgressRetryInterval
	snap.EgressMaxRetryInterval = settings.EgressMaxRetryInterval
	snap.EgressMaxAge = settings.EgressMaxAge
	snap.BounceDomain = settings.BounceDomain
	snap.FBLDomains = settings.FBLDomains
	snap.BounceClassifierFile = settings.BounceClassifierFile
	snap.BounceVerpSecret = settings.BounceVerpSecret
	return RenderKumoConfig(snap)
}

// ApplyResult is returned after a config apply.
type ApplyResult struct {
	RequestID     string
	Status        string
	Checksum      string
	AppliedPath   string
	ResultSummary string
	// Restarted is true when the apply required a KumoMTA restart (init change)
	// rather than a hot reload.
	Restarted bool
}

// Apply renders the configuration, writes it to KumoMTA, and reloads the
// service. It is serialized: only one service-control/apply operation may be in
// flight at a time. Every attempt is audited.
func (uc *KumoConfigUsecase) Apply(ctx context.Context, confirmationID string) (*ApplyResult, error) {
	id, err := RequirePermission(ctx, PermServiceControl)
	if err != nil {
		uc.audit(ctx, AuditDenied, "", map[string]any{"reason": "permission_denied"})
		return nil, err
	}
	if strings.TrimSpace(confirmationID) == "" {
		return nil, Invalid("CONFIRMATION_REQUIRED", "confirmation_id is required to apply configuration")
	}

	active, err := uc.scStore.ActiveServiceControlExists(ctx)
	if err != nil {
		return nil, err
	}
	if active {
		return nil, Conflict("SERVICE_CONTROL_ACTIVE", "another service-control operation is already in progress")
	}

	rendered, err := uc.render(ctx)
	if err != nil {
		return nil, err
	}
	// Never apply a policy that does not lint as valid Lua — this is the last
	// guard against an escaping defect or a malformed entity reaching KumoMTA.
	if !rendered.Valid {
		return nil, FailedPrecondition("CONFIG_LINT_FAILED",
			"generated policy failed Lua lint: %s", strings.Join(rendered.LintIssues, "; "))
	}

	rec, err := uc.scStore.CreateServiceControlRequest(ctx, &ServiceControlRecord{
		Operation: "config.apply", ConfirmationID: confirmationID, RequestedBy: id.UserID,
	})
	if err != nil {
		uc.audit(ctx, AuditFailure, "", map[string]any{"checksum": rendered.Checksum})
		return nil, err
	}

	if err := uc.scStore.UpdateServiceControlStatus(ctx, rec.ID, SvcRunning, ""); err != nil {
		LoggerFrom(ctx).Error("mark config apply running", "error", err.Error())
	}

	// A change to the init block (listeners/spool/log hook) needs a restart, not
	// just a reload, since KumoMTA only runs kumo.on('init') at startup.
	appliedChecksum, appliedInit, _, csErr := uc.scStore.GetAppliedChecksum(ctx)
	if csErr != nil {
		return nil, csErr
	}
	restart := appliedChecksum == "" || appliedInit != rendered.InitChecksum

	path, summary, applyErr := uc.kumo.ApplyConfig(ctx, rendered, restart)
	status := SvcSucceeded
	if applyErr != nil {
		status = SvcFailed
		summary = applyErr.Error()
	}
	if err := uc.scStore.UpdateServiceControlStatus(ctx, rec.ID, status, summary); err != nil {
		LoggerFrom(ctx).Error("mark config apply terminal", "error", err.Error())
	}

	outcome := AuditSuccess
	if applyErr != nil {
		outcome = AuditFailure
	}
	uc.audit(ctx, outcome, rec.ID, map[string]any{
		"checksum": rendered.Checksum, "applied_path": path,
		"vmta_count": rendered.VMTACount, "pool_count": rendered.PoolCount,
		"route_count": rendered.RouteCount, "dkim_count": rendered.DKIMCount,
		"suppression_count": rendered.SuppressionCount,
	})

	if applyErr != nil {
		return nil, applyErr
	}
	// Record the applied checksums so the UI can detect later config drift and
	// whether the next change needs a restart.
	if err := uc.scStore.SetAppliedChecksum(ctx, rendered.Checksum, rendered.InitChecksum, id.UserID); err != nil {
		LoggerFrom(ctx).Error("record applied checksum", "error", err.Error())
	}
	return &ApplyResult{
		RequestID: rec.ID, Status: status, Checksum: rendered.Checksum,
		AppliedPath: path, ResultSummary: summary, Restarted: restart,
	}, nil
}

// ConfigStatus describes whether the current configuration has drifted from the
// last applied KumoMTA policy.
type ConfigStatus struct {
	Drift           bool
	NeverApplied    bool
	CurrentChecksum string
	AppliedChecksum string
	AppliedAt       *time.Time
	// RestartRequired is true when the pending change touches the init block, so
	// applying it needs a KumoMTA restart rather than a reload.
	RestartRequired bool
}

// Status renders the current configuration and compares its checksum to the
// last applied one, reporting whether a regenerate/apply is pending and whether
// applying it requires a restart (init change) rather than a reload.
func (uc *KumoConfigUsecase) Status(ctx context.Context) (*ConfigStatus, error) {
	if _, err := RequirePermission(ctx, PermVMTARead); err != nil {
		return nil, err
	}
	rendered, err := uc.render(ctx)
	if err != nil {
		return nil, err
	}
	applied, appliedInit, appliedAt, err := uc.scStore.GetAppliedChecksum(ctx)
	if err != nil {
		return nil, err
	}
	never := applied == ""
	drift := never || applied != rendered.Checksum
	return &ConfigStatus{
		Drift:           drift,
		NeverApplied:    never,
		CurrentChecksum: rendered.Checksum,
		AppliedChecksum: applied,
		AppliedAt:       appliedAt,
		// A restart is needed when there's drift and the init block differs (or
		// nothing has been applied yet).
		RestartRequired: drift && (never || appliedInit != rendered.InitChecksum),
	}, nil
}

func (uc *KumoConfigUsecase) audit(ctx context.Context, outcome AuditOutcome, targetID string, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, "kumomta.config.apply", "kumomta", targetID, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", "kumomta.config.apply", "error", err.Error())
	}
}
