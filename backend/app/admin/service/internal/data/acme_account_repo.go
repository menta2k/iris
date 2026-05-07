// AcmeAccountRepo backs service.AcmeAccountStore — singleton row,
// id=1, seeded on first read same as global_settings.
package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

const acmeAccountID = 1

type AcmeAccountRepo struct{ client *ent.Client }

func NewAcmeAccountRepo(c *ent.Client) *AcmeAccountRepo { return &AcmeAccountRepo{client: c} }

func (r *AcmeAccountRepo) Get(ctx context.Context) (*service.AcmeAccountRow, error) {
	row, err := r.client.AcmeAccount.Get(ctx, acmeAccountID)
	if err != nil {
		if !ent.IsNotFound(err) {
			return nil, fmt.Errorf("acme_account_repo: get: %w", err)
		}
		if _, err := r.client.AcmeAccount.Create().SetID(acmeAccountID).Save(ctx); err != nil {
			return nil, fmt.Errorf("acme_account_repo: seed: %w", err)
		}
		row, err = r.client.AcmeAccount.Get(ctx, acmeAccountID)
		if err != nil {
			return nil, err
		}
	}
	return acmeAccountToRow(row), nil
}

func (r *AcmeAccountRepo) Save(ctx context.Context, in service.AcmeAccountRow) (*service.AcmeAccountRow, error) {
	if _, err := r.client.AcmeAccount.Get(ctx, acmeAccountID); err != nil {
		if !ent.IsNotFound(err) {
			return nil, fmt.Errorf("acme_account_repo: lookup: %w", err)
		}
		if _, err := r.client.AcmeAccount.Create().SetID(acmeAccountID).Save(ctx); err != nil {
			return nil, fmt.Errorf("acme_account_repo: seed: %w", err)
		}
	}
	saved, err := r.client.AcmeAccount.UpdateOneID(acmeAccountID).
		SetEmail(in.Email).
		SetServerURL(in.ServerURL).
		SetRegistrationJSON(in.RegistrationJSON).
		SetPrivateKeyPem(in.PrivateKeyPEM).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("acme_account_repo: save: %w", err)
	}
	return acmeAccountToRow(saved), nil
}

func acmeAccountToRow(g *ent.AcmeAccount) *service.AcmeAccountRow {
	if g == nil {
		return &service.AcmeAccountRow{}
	}
	return &service.AcmeAccountRow{
		Email:            g.Email,
		ServerURL:        g.ServerURL,
		HasRegistration:  g.RegistrationJSON != "",
		RegistrationJSON: g.RegistrationJSON,
		PrivateKeyPEM:    g.PrivateKeyPem,
		UpdatedAt:        g.UpdatedAt,
	}
}

var _ service.AcmeAccountStore = (*AcmeAccountRepo)(nil)
