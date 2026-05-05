// SuppressionRepo backs service.SuppressionStore with ent.
package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/suppressionentry"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// timeNow is var-form so tests can freeze it; production callers use the
// real wall clock.
var timeNow = func() time.Time { return time.Now() }

// SuppressionRepo persists suppression entries.
type SuppressionRepo struct{ client *ent.Client }

// NewSuppressionRepo wires the ent client.
func NewSuppressionRepo(c *ent.Client) *SuppressionRepo { return &SuppressionRepo{client: c} }

// List returns the most recent entries first.
func (r *SuppressionRepo) List(ctx context.Context, limit, offset int) ([]service.SuppressionRow, uint32, error) {
	total, err := r.client.SuppressionEntry.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("suppression_repo: count: %w", err)
	}
	rows, err := r.client.SuppressionEntry.Query().
		Order(ent.Desc(suppressionentry.FieldCreatedAt)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("suppression_repo: list: %w", err)
	}
	out := make([]service.SuppressionRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, suppToRow(e))
	}
	return out, uint32(total), nil
}

// Get one entry by id.
func (r *SuppressionRepo) Get(ctx context.Context, id uint64) (*service.SuppressionRow, error) {
	e, err := r.client.SuppressionEntry.Get(ctx, int(id))
	if err != nil {
		return nil, fmt.Errorf("suppression_repo: get: %w", err)
	}
	row := suppToRow(e)
	return &row, nil
}

// Upsert creates or refreshes by (address, scope). The unique index on
// (address, scope) means a duplicate INSERT is detected and we fall back to
// UpdateOne on the existing row.
func (r *SuppressionRepo) Upsert(ctx context.Context, row *service.SuppressionRow) (*service.SuppressionRow, error) {
	existing, err := r.client.SuppressionEntry.Query().
		Where(suppressionentry.AddressEQ(row.Address), suppressionentry.ScopeEQ(row.Scope)).
		Only(ctx)
	if err == nil {
		upd := r.client.SuppressionEntry.UpdateOneID(existing.ID).
			SetReason(row.Reason).
			SetNote(row.Note)
		if row.ExpiresAt != nil {
			upd = upd.SetExpiresAt(*row.ExpiresAt)
		} else {
			upd = upd.ClearExpiresAt()
		}
		updated, err := upd.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("suppression_repo: update: %w", err)
		}
		out := suppToRow(updated)
		return &out, nil
	}
	if !errors.Is(err, &ent.NotFoundError{}) && !ent.IsNotFound(err) {
		return nil, fmt.Errorf("suppression_repo: lookup: %w", err)
	}
	c := r.client.SuppressionEntry.Create().
		SetAddress(row.Address).
		SetScope(row.Scope).
		SetReason(row.Reason).
		SetNote(row.Note).
		SetCreatedAt(row.CreatedAt)
	if row.ExpiresAt != nil {
		c = c.SetExpiresAt(*row.ExpiresAt)
	}
	created, err := c.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("suppression_repo: create: %w", err)
	}
	out := suppToRow(created)
	return &out, nil
}

// Delete removes by id.
func (r *SuppressionRepo) Delete(ctx context.Context, id uint64) error {
	if err := r.client.SuppressionEntry.DeleteOneID(int(id)).Exec(ctx); err != nil {
		return fmt.Errorf("suppression_repo: delete: %w", err)
	}
	return nil
}

// IterateAll streams (address, scope) tuples for every active entry,
// invoking yield for each. Used by the suppressionindex resync to rebuild
// the Redis SETs from PG. Pages of `pageSize` so a multi-million-row
// table doesn't materialise in memory.
//
// Expired rows (ExpiresAt < now) are filtered out at the SQL layer so
// the index doesn't carry rows kumomta would no longer enforce.
func (r *SuppressionRepo) IterateAll(ctx context.Context, pageSize int, yield func(address, scope string) error) error {
	if pageSize <= 0 || pageSize > 50_000 {
		pageSize = 5_000
	}
	offset := 0
	for {
		// Order by ID for stable pagination — created_at would tie under
		// bulk imports and risk skipping rows with identical timestamps.
		page, err := r.client.SuppressionEntry.Query().
			Where(
				suppressionentry.Or(
					suppressionentry.ExpiresAtIsNil(),
					suppressionentry.ExpiresAtGT(timeNow()),
				),
			).
			Order(ent.Asc(suppressionentry.FieldID)).
			Offset(offset).Limit(pageSize).
			All(ctx)
		if err != nil {
			return fmt.Errorf("suppression_repo: iterate: %w", err)
		}
		if len(page) == 0 {
			return nil
		}
		for _, e := range page {
			if err := yield(e.Address, e.Scope); err != nil {
				return err
			}
		}
		if len(page) < pageSize {
			return nil
		}
		offset += pageSize
	}
}

func suppToRow(e *ent.SuppressionEntry) service.SuppressionRow {
	return service.SuppressionRow{
		ID:        uint64(e.ID),
		Address:   e.Address,
		Scope:     e.Scope,
		Reason:    e.Reason,
		Note:      e.Note,
		CreatedAt: e.CreatedAt,
		ExpiresAt: e.ExpiresAt,
	}
}

var _ service.SuppressionStore = (*SuppressionRepo)(nil)
