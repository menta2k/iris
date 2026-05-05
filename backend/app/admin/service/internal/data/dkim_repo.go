// DkimRepo backs service.DkimStore with ent.
package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/dkimidentity"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// DkimRepo persists DKIM identities.
type DkimRepo struct{ client *ent.Client }

// NewDkimRepo wires the ent client.
func NewDkimRepo(c *ent.Client) *DkimRepo { return &DkimRepo{client: c} }

func (r *DkimRepo) List(ctx context.Context, limit, offset int) ([]service.DkimRow, uint32, error) {
	total, err := r.client.DkimIdentity.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("dkim_repo: count: %w", err)
	}
	rows, err := r.client.DkimIdentity.Query().
		Order(ent.Asc(dkimidentity.FieldDomain), ent.Asc(dkimidentity.FieldSelector)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("dkim_repo: list: %w", err)
	}
	out := make([]service.DkimRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, dkimToRow(e))
	}
	return out, uint32(total), nil
}

func (r *DkimRepo) Get(ctx context.Context, id uint32) (*service.DkimRow, error) {
	e, err := r.client.DkimIdentity.Get(ctx, int(id))
	if err != nil {
		return nil, fmt.Errorf("dkim_repo: get: %w", err)
	}
	row := dkimToRow(e)
	return &row, nil
}

func (r *DkimRepo) Create(ctx context.Context, in service.DkimRow) (*service.DkimRow, error) {
	created, err := r.client.DkimIdentity.Create().
		SetDomain(in.Domain).
		SetSelector(in.Selector).
		SetAlgorithm(in.Algorithm).
		SetPublicKeyPem(in.PublicKeyPEM).
		SetKeyPath(in.KeyPath).
		SetActive(in.Active).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("dkim_repo: create: %w", err)
	}
	out := dkimToRow(created)
	return &out, nil
}

func (r *DkimRepo) UpdateKey(ctx context.Context, id uint32, publicPEM, keyPath, algorithm string) (*service.DkimRow, error) {
	updated, err := r.client.DkimIdentity.UpdateOneID(int(id)).
		SetPublicKeyPem(publicPEM).
		SetKeyPath(keyPath).
		SetAlgorithm(algorithm).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("dkim_repo: update key: %w", err)
	}
	out := dkimToRow(updated)
	return &out, nil
}

func (r *DkimRepo) Delete(ctx context.Context, id uint32) error {
	if err := r.client.DkimIdentity.DeleteOneID(int(id)).Exec(ctx); err != nil {
		return fmt.Errorf("dkim_repo: delete: %w", err)
	}
	return nil
}

func dkimToRow(e *ent.DkimIdentity) service.DkimRow {
	return service.DkimRow{
		ID:           uint32(e.ID),
		Domain:       e.Domain,
		Selector:     e.Selector,
		Algorithm:    e.Algorithm,
		PublicKeyPEM: e.PublicKeyPem,
		KeyPath:      e.KeyPath,
		Active:       e.Active,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
}

var _ service.DkimStore = (*DkimRepo)(nil)
