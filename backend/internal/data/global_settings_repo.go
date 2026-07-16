package data

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/menta2k/iris/backend/internal/biz"
)

// GlobalSettingsRepo persists the singleton global_settings row.
type GlobalSettingsRepo struct {
	db *DB
}

// NewGlobalSettingsRepo constructs the repository.
func NewGlobalSettingsRepo(db *DB) *GlobalSettingsRepo { return &GlobalSettingsRepo{db: db} }

var _ biz.GlobalSettingsRepo = (*GlobalSettingsRepo)(nil)

const globalSettingsCols = `rspamd_mode, rspamd_url, egress_ehlo_domain,
	log_stream_redis_url, esmtp_listen, http_listen,
	egress_retry_interval, egress_max_retry_interval, egress_max_age,
	bounce_domain, auto_suppress_hard_bounces, soft_bounce_threshold,
	suppression_ttl, dmarc_report_email, admin_http_addr, admin_tls_enabled, admin_tls_cert_domain,
	acme_renew_interval, acme_renew_before, prometheus_url, fbl_require_verification,
	inbound_maildir_base_path, bounce_domain_template,
	classify_subjects, classify_model, classify_threshold, classify_api_base,
	pin_egress_per_message,
	injection_enabled, injection_listen_addr, injection_path, injection_tls_enabled, injection_tls_cert_domain,
	monitoring_from, monitoring_reconcile_lookback, monitoring_fetch_timeout, monitoring_fetch_giveup,
	tls_auto_disable,
	updated_at, updated_by`

// scanGlobalSettings scans a row in globalSettingsCols order.
func scanGlobalSettings(row interface{ Scan(...any) error }) (*biz.GlobalSettings, error) {
	out := &biz.GlobalSettings{}
	err := row.Scan(&out.RspamdMode, &out.RspamdURL, &out.EgressEHLODomain,
		&out.LogStreamRedisURL, &out.EsmtpListen, &out.HTTPListen,
		&out.EgressRetryInterval, &out.EgressMaxRetryInterval, &out.EgressMaxAge,
		&out.BounceDomain, &out.AutoSuppressHardBounces, &out.SoftBounceThreshold,
		&out.SuppressionTTL, &out.DMARCReportEmail, &out.AdminHTTPAddr, &out.AdminTLSEnabled, &out.AdminTLSCertDomain,
		&out.AcmeRenewInterval, &out.AcmeRenewBefore, &out.PrometheusURL, &out.FBLRequireVerification,
		&out.InboundMaildirBasePath, &out.BounceDomainTemplate,
		&out.ClassifySubjects, &out.ClassifyModel, &out.ClassifyThreshold, &out.ClassifyAPIBase,
		&out.PinEgressPerMessage,
		&out.InjectionEnabled, &out.InjectionListenAddr, &out.InjectionPath, &out.InjectionTLSEnabled, &out.InjectionTLSCertDomain,
		&out.MonitoringFrom, &out.MonitoringReconcileLookback, &out.MonitoringFetchTimeout, &out.MonitoringFetchGiveUp,
		&out.TLSAutoDisable,
		&out.UpdatedAt, &out.UpdatedBy)
	return out, err
}

// Get returns the singleton row, seeding it if the migration's seed is missing.
func (r *GlobalSettingsRepo) Get(ctx context.Context) (*biz.GlobalSettings, error) {
	out, err := scanGlobalSettings(r.db.Pool.QueryRow(ctx,
		`SELECT `+globalSettingsCols+` FROM global_settings WHERE id = 1`))
	if err == pgx.ErrNoRows {
		if _, ierr := r.db.Pool.Exec(ctx, `INSERT INTO global_settings (id) VALUES (1) ON CONFLICT DO NOTHING`); ierr != nil {
			return nil, fmt.Errorf("seed global settings: %w", ierr)
		}
		return &biz.GlobalSettings{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get global settings: %w", err)
	}
	return out, nil
}

// Update writes every field on the singleton row and returns the stored state.
func (r *GlobalSettingsRepo) Update(ctx context.Context, in *biz.GlobalSettings, actor string) (*biz.GlobalSettings, error) {
	out, err := scanGlobalSettings(r.db.Pool.QueryRow(ctx, `
		UPDATE global_settings SET
			rspamd_mode = $1, rspamd_url = $2, egress_ehlo_domain = $3,
			log_stream_redis_url = $4, esmtp_listen = $5, http_listen = $6,
			egress_retry_interval = $7, egress_max_retry_interval = $8, egress_max_age = $9,
			bounce_domain = $10, auto_suppress_hard_bounces = $11, soft_bounce_threshold = $12,
			suppression_ttl = $13, dmarc_report_email = $14, admin_http_addr = $15, admin_tls_enabled = $16,
			admin_tls_cert_domain = $17, acme_renew_interval = $18, acme_renew_before = $19,
			prometheus_url = $20, fbl_require_verification = $21,
			inbound_maildir_base_path = $22, bounce_domain_template = $23,
			classify_subjects = $24, classify_model = $25, classify_threshold = $26, classify_api_base = $27,
			pin_egress_per_message = $28,
			injection_enabled = $29, injection_listen_addr = $30, injection_path = $31,
			injection_tls_enabled = $32, injection_tls_cert_domain = $33,
			monitoring_from = $34, monitoring_reconcile_lookback = $35,
			monitoring_fetch_timeout = $36, monitoring_fetch_giveup = $37,
			tls_auto_disable = $38,
			updated_at = now(), updated_by = $39
		WHERE id = 1
		RETURNING `+globalSettingsCols,
		in.RspamdMode, in.RspamdURL, in.EgressEHLODomain,
		in.LogStreamRedisURL, in.EsmtpListen, in.HTTPListen,
		in.EgressRetryInterval, in.EgressMaxRetryInterval, in.EgressMaxAge,
		in.BounceDomain, in.AutoSuppressHardBounces, in.SoftBounceThreshold,
		in.SuppressionTTL, in.DMARCReportEmail, in.AdminHTTPAddr, in.AdminTLSEnabled, in.AdminTLSCertDomain,
		in.AcmeRenewInterval, in.AcmeRenewBefore, in.PrometheusURL, in.FBLRequireVerification,
		in.InboundMaildirBasePath, in.BounceDomainTemplate,
		in.ClassifySubjects, in.ClassifyModel, in.ClassifyThreshold, in.ClassifyAPIBase,
		in.PinEgressPerMessage,
		in.InjectionEnabled, in.InjectionListenAddr, in.InjectionPath, in.InjectionTLSEnabled, in.InjectionTLSCertDomain,
		in.MonitoringFrom, in.MonitoringReconcileLookback, in.MonitoringFetchTimeout, in.MonitoringFetchGiveUp,
		in.TLSAutoDisable,
		actor))
	if err != nil {
		return nil, mapConstraint(err, "global_settings")
	}
	return out, nil
}
