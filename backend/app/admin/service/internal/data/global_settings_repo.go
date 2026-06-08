// GlobalSettingsRepo backs service.GlobalSettingsStore with ent. The
// table is a singleton (id=1, enforced by a CHECK constraint in
// sql/0006_global_settings.sql); every Get / Update operates on that
// row, seeding it if the migration didn't (e.g. fresh ent-only schema
// diff on a brand-new DB).
package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

const globalSettingsID = 1

// GlobalSettingsRepo persists/reads the singleton settings row.
type GlobalSettingsRepo struct{ client *ent.Client }

// NewGlobalSettingsRepo wires the ent client.
func NewGlobalSettingsRepo(c *ent.Client) *GlobalSettingsRepo {
	return &GlobalSettingsRepo{client: c}
}

// Get returns the row, seeding it if missing. The seed path matters for
// fresh databases that came up via ent's schema.Create without our SQL
// migration (e.g. tests).
func (r *GlobalSettingsRepo) Get(ctx context.Context) (*service.GlobalSettingsRow, error) {
	row, err := r.client.GlobalSettings.Get(ctx, globalSettingsID)
	if err != nil {
		if !ent.IsNotFound(err) {
			return nil, fmt.Errorf("global_settings_repo: get: %w", err)
		}
		// Seed and re-fetch.
		if _, err := r.client.GlobalSettings.Create().SetID(globalSettingsID).Save(ctx); err != nil {
			return nil, fmt.Errorf("global_settings_repo: seed: %w", err)
		}
		row, err = r.client.GlobalSettings.Get(ctx, globalSettingsID)
		if err != nil {
			return nil, fmt.Errorf("global_settings_repo: re-get: %w", err)
		}
	}
	return entToRow(row), nil
}

// Update writes every UI-managed field. The actor name is denormalised
// onto updated_by so the audit log can answer "who changed what" without
// a join.
func (r *GlobalSettingsRepo) Update(ctx context.Context, in service.GlobalSettingsRow, actor string) (*service.GlobalSettingsRow, error) {
	if _, err := r.client.GlobalSettings.Get(ctx, globalSettingsID); err != nil {
		if !ent.IsNotFound(err) {
			return nil, fmt.Errorf("global_settings_repo: lookup before update: %w", err)
		}
		// Seed empty so the UpdateOneID below has a row to mutate.
		if _, err := r.client.GlobalSettings.Create().SetID(globalSettingsID).Save(ctx); err != nil {
			return nil, fmt.Errorf("global_settings_repo: seed before update: %w", err)
		}
	}
	upd := r.client.GlobalSettings.UpdateOneID(globalSettingsID).
		SetKumoHTTPListen(in.KumoHTTPListen).
		SetEsmtpListenAddr(in.EsmtpListenAddr).
		SetEsmtpRelayHosts(append([]string(nil), in.EsmtpRelayHosts...)).
		SetHTTPTrustedHosts(append([]string(nil), in.HTTPTrustedHosts...)).
		SetBounceDomain(in.BounceDomain).
		SetBounceSenderDomains(append([]string(nil), in.BounceSenderDomains...)).
		SetBouncePrefix(in.BouncePrefix).
		SetMailClassHeader(in.MailClassHeader).
		SetEgressEhloDomain(in.EgressEhloDomain).
		SetEgressRetryInterval(in.EgressRetryInterval).
		SetEgressMaxRetryInterval(in.EgressMaxRetryInterval).
		SetEgressMaxAge(in.EgressMaxAge).
		SetRspamdMode(in.RspamdMode).
		SetRspamdURL(in.RspamdURL).
		SetHTTPSListen(in.HTTPSListen).
		SetHTTPSCertPemPath(in.HTTPSCertPemPath).
		SetHTTPSKeyPemPath(in.HTTPSKeyPemPath).
		SetUpdatedBy(actor)
	saved, err := upd.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("global_settings_repo: update: %w", err)
	}
	return entToRow(saved), nil
}

func entToRow(g *ent.GlobalSettings) *service.GlobalSettingsRow {
	if g == nil {
		return nil
	}
	return &service.GlobalSettingsRow{
		KumoHTTPListen:      g.KumoHTTPListen,
		EsmtpListenAddr:     g.EsmtpListenAddr,
		EsmtpRelayHosts:     append([]string(nil), g.EsmtpRelayHosts...),
		HTTPTrustedHosts:    append([]string(nil), g.HTTPTrustedHosts...),
		BounceDomain:        g.BounceDomain,
		BounceSenderDomains: append([]string(nil), g.BounceSenderDomains...),
		BouncePrefix:        g.BouncePrefix,
		MailClassHeader:        g.MailClassHeader,
		EgressEhloDomain:       g.EgressEhloDomain,
		EgressRetryInterval:    g.EgressRetryInterval,
		EgressMaxRetryInterval: g.EgressMaxRetryInterval,
		EgressMaxAge:           g.EgressMaxAge,
		RspamdMode:             g.RspamdMode,
		RspamdURL:              g.RspamdURL,
		HTTPSListen:            g.HTTPSListen,
		HTTPSCertPemPath:    g.HTTPSCertPemPath,
		HTTPSKeyPemPath:     g.HTTPSKeyPemPath,
		UpdatedAt:           g.UpdatedAt,
		UpdatedBy:           g.UpdatedBy,
	}
}

// Compile-time check that the repo satisfies the service-layer iface so
// a future signature change surfaces here, not at wire-construction.
var _ service.GlobalSettingsStore = (*GlobalSettingsRepo)(nil)

// Force compile-time use of errors so future maintenance can wrap with %w.
var _ = errors.New
