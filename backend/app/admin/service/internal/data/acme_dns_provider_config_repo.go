// AcmeDnsProviderConfigRepo: per-provider saved credentials for DNS-01.
package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/acmednsproviderconfig"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

type AcmeDnsProviderConfigRepo struct{ client *ent.Client }

func NewAcmeDnsProviderConfigRepo(c *ent.Client) *AcmeDnsProviderConfigRepo {
	return &AcmeDnsProviderConfigRepo{client: c}
}

func (r *AcmeDnsProviderConfigRepo) List(ctx context.Context) ([]service.AcmeDnsProviderConfigRow, error) {
	rows, err := r.client.AcmeDnsProviderConfig.Query().
		Order(ent.Asc(acmednsproviderconfig.FieldProvider)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("acme_dns_provider_config_repo: list: %w", err)
	}
	out := make([]service.AcmeDnsProviderConfigRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, dnsCfgToRow(e))
	}
	return out, nil
}

func (r *AcmeDnsProviderConfigRepo) Get(ctx context.Context, provider string) (*service.AcmeDnsProviderConfigRow, error) {
	e, err := r.client.AcmeDnsProviderConfig.Query().
		Where(acmednsproviderconfig.ProviderEQ(provider)).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("acme_dns_provider_config_repo: get %s: %w", provider, err)
	}
	row := dnsCfgToRow(e)
	return &row, nil
}

func (r *AcmeDnsProviderConfigRepo) Upsert(ctx context.Context, in service.AcmeDnsProviderConfigRow, actor string) (*service.AcmeDnsProviderConfigRow, error) {
	cfgJSON, err := json.Marshal(in.Config)
	if err != nil {
		return nil, fmt.Errorf("acme_dns_provider_config_repo: marshal config: %w", err)
	}
	existing, err := r.client.AcmeDnsProviderConfig.Query().
		Where(acmednsproviderconfig.ProviderEQ(in.Provider)).
		Only(ctx)
	if err == nil {
		saved, err := r.client.AcmeDnsProviderConfig.UpdateOneID(existing.ID).
			SetConfigJSON(string(cfgJSON)).
			SetUpdatedBy(actor).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("acme_dns_provider_config_repo: update: %w", err)
		}
		row := dnsCfgToRow(saved)
		return &row, nil
	}
	if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("acme_dns_provider_config_repo: upsert lookup: %w", err)
	}
	saved, err := r.client.AcmeDnsProviderConfig.Create().
		SetProvider(in.Provider).
		SetConfigJSON(string(cfgJSON)).
		SetUpdatedBy(actor).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("acme_dns_provider_config_repo: create: %w", err)
	}
	row := dnsCfgToRow(saved)
	return &row, nil
}

func (r *AcmeDnsProviderConfigRepo) Delete(ctx context.Context, provider string) error {
	_, err := r.client.AcmeDnsProviderConfig.Delete().
		Where(acmednsproviderconfig.ProviderEQ(provider)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("acme_dns_provider_config_repo: delete: %w", err)
	}
	return nil
}

func dnsCfgToRow(g *ent.AcmeDnsProviderConfig) service.AcmeDnsProviderConfigRow {
	cfg := map[string]string{}
	if g.ConfigJSON != "" {
		_ = json.Unmarshal([]byte(g.ConfigJSON), &cfg)
	}
	return service.AcmeDnsProviderConfigRow{
		Provider:  g.Provider,
		Config:    cfg,
		UpdatedAt: g.UpdatedAt,
		UpdatedBy: g.UpdatedBy,
	}
}

var _ service.AcmeDnsProviderConfigStore = (*AcmeDnsProviderConfigRepo)(nil)
