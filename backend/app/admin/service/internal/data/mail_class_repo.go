// MailClassRepo backs service.MailClassStore with ent.
package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/mailclass"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// MailClassRepo persists header-driven mail-class shortcuts.
type MailClassRepo struct{ client *ent.Client }

// NewMailClassRepo wires the ent client.
func NewMailClassRepo(c *ent.Client) *MailClassRepo { return &MailClassRepo{client: c} }

// List returns rows ordered by name.
func (r *MailClassRepo) List(ctx context.Context, limit, offset int) ([]service.MailClassRow, uint32, error) {
	total, err := r.client.MailClass.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("mail_class_repo: count: %w", err)
	}
	rows, err := r.client.MailClass.Query().
		Order(ent.Asc(mailclass.FieldName)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("mail_class_repo: list: %w", err)
	}
	out := make([]service.MailClassRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, mailClassToRow(e))
	}
	return out, uint32(total), nil
}

// Get one row by id.
func (r *MailClassRepo) Get(ctx context.Context, id uint32) (*service.MailClassRow, error) {
	e, err := r.client.MailClass.Get(ctx, int(id))
	if err != nil {
		return nil, fmt.Errorf("mail_class_repo: get: %w", err)
	}
	row := mailClassToRow(e)
	return &row, nil
}

// Create persists a new mail class.
func (r *MailClassRepo) Create(ctx context.Context, in service.MailClassRow) (*service.MailClassRow, error) {
	e, err := r.client.MailClass.Create().
		SetName(in.Name).
		SetDescription(in.Description).
		SetEnabled(in.Enabled).
		SetTargetKind(in.TargetKind).
		SetTargetRef(in.TargetRef).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("mail_class_repo: create: %w", err)
	}
	row := mailClassToRow(e)
	return &row, nil
}

// Update overwrites mutable fields.
func (r *MailClassRepo) Update(ctx context.Context, id uint32, in service.MailClassRow) (*service.MailClassRow, error) {
	e, err := r.client.MailClass.UpdateOneID(int(id)).
		SetName(in.Name).
		SetDescription(in.Description).
		SetEnabled(in.Enabled).
		SetTargetKind(in.TargetKind).
		SetTargetRef(in.TargetRef).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("mail_class_repo: update: %w", err)
	}
	row := mailClassToRow(e)
	return &row, nil
}

// Delete removes by id.
func (r *MailClassRepo) Delete(ctx context.Context, id uint32) error {
	if err := r.client.MailClass.DeleteOneID(int(id)).Exec(ctx); err != nil {
		return fmt.Errorf("mail_class_repo: delete: %w", err)
	}
	return nil
}

func mailClassToRow(e *ent.MailClass) service.MailClassRow {
	return service.MailClassRow{
		ID:          uint32(e.ID),
		Name:        e.Name,
		Description: e.Description,
		Enabled:     e.Enabled,
		TargetKind:  e.TargetKind,
		TargetRef:   e.TargetRef,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

var _ service.MailClassStore = (*MailClassRepo)(nil)
