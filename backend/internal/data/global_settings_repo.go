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
	fbl_domains, admin_http_addr, admin_tls_enabled, admin_tls_cert_domain,
	acme_renew_interval, acme_renew_before, prometheus_url, updated_at, updated_by`

// scanGlobalSettings scans a row in globalSettingsCols order.
func scanGlobalSettings(row interface{ Scan(...any) error }) (*biz.GlobalSettings, error) {
	out := &biz.GlobalSettings{}
	err := row.Scan(&out.RspamdMode, &out.RspamdURL, &out.EgressEHLODomain,
		&out.LogStreamRedisURL, &out.EsmtpListen, &out.HTTPListen,
		&out.EgressRetryInterval, &out.EgressMaxRetryInterval, &out.EgressMaxAge,
		&out.BounceDomain, &out.AutoSuppressHardBounces, &out.SoftBounceThreshold,
		&out.FBLDomains, &out.AdminHTTPAddr, &out.AdminTLSEnabled, &out.AdminTLSCertDomain,
		&out.AcmeRenewInterval, &out.AcmeRenewBefore, &out.PrometheusURL, &out.UpdatedAt, &out.UpdatedBy)
	out.FBLDomains = nonNilStrings(out.FBLDomains)
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
			fbl_domains = $13, admin_http_addr = $14, admin_tls_enabled = $15,
			admin_tls_cert_domain = $16, acme_renew_interval = $17, acme_renew_before = $18,
			prometheus_url = $19, updated_at = now(), updated_by = $20
		WHERE id = 1
		RETURNING `+globalSettingsCols,
		in.RspamdMode, in.RspamdURL, in.EgressEHLODomain,
		in.LogStreamRedisURL, in.EsmtpListen, in.HTTPListen,
		in.EgressRetryInterval, in.EgressMaxRetryInterval, in.EgressMaxAge,
		in.BounceDomain, in.AutoSuppressHardBounces, in.SoftBounceThreshold,
		in.FBLDomains, in.AdminHTTPAddr, in.AdminTLSEnabled, in.AdminTLSCertDomain,
		in.AcmeRenewInterval, in.AcmeRenewBefore, in.PrometheusURL, actor))
	if err != nil {
		return nil, mapConstraint(err, "global_settings")
	}
	return out, nil
}
