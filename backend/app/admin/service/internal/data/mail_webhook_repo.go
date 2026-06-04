// MailWebhookRepo backs service.MailWebhookStore with ent.
package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/mailwebhook"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// MailWebhookRepo persists inbound-mail → HTTP webhook rows.
type MailWebhookRepo struct{ client *ent.Client }

func NewMailWebhookRepo(c *ent.Client) *MailWebhookRepo { return &MailWebhookRepo{client: c} }

func (r *MailWebhookRepo) List(ctx context.Context, limit, offset int) ([]service.MailWebhookRow, uint32, error) {
	total, err := r.client.MailWebhook.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("mail_webhook_repo: count: %w", err)
	}
	rows, err := r.client.MailWebhook.Query().
		Order(ent.Asc(mailwebhook.FieldName)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("mail_webhook_repo: list: %w", err)
	}
	out := make([]service.MailWebhookRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, mailWebhookToRow(e))
	}
	return out, uint32(total), nil
}

func (r *MailWebhookRepo) Get(ctx context.Context, id uint32) (*service.MailWebhookRow, error) {
	e, err := r.client.MailWebhook.Get(ctx, int(id))
	if err != nil {
		return nil, fmt.Errorf("mail_webhook_repo: get: %w", err)
	}
	row := mailWebhookToRow(e)
	return &row, nil
}

func (r *MailWebhookRepo) Create(ctx context.Context, in service.MailWebhookRow) (*service.MailWebhookRow, error) {
	created, err := r.client.MailWebhook.Create().
		SetName(in.Name).
		SetAddress(in.Address).
		SetURL(in.URL).
		SetSecret(in.Secret).
		SetEnabled(in.Enabled).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("mail_webhook_repo: create: %w", err)
	}
	out := mailWebhookToRow(created)
	return &out, nil
}

// Update mutates an existing webhook. Name is immutable (delete + recreate to
// rename), matching the Listener/VMTA convention.
func (r *MailWebhookRepo) Update(ctx context.Context, id uint32, in service.MailWebhookRow) (*service.MailWebhookRow, error) {
	updated, err := r.client.MailWebhook.UpdateOneID(int(id)).
		SetAddress(in.Address).
		SetURL(in.URL).
		SetSecret(in.Secret).
		SetEnabled(in.Enabled).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("mail_webhook_repo: update: %w", err)
	}
	out := mailWebhookToRow(updated)
	return &out, nil
}

func (r *MailWebhookRepo) Delete(ctx context.Context, id uint32) error {
	if err := r.client.MailWebhook.DeleteOneID(int(id)).Exec(ctx); err != nil {
		return fmt.Errorf("mail_webhook_repo: delete: %w", err)
	}
	return nil
}

func mailWebhookToRow(e *ent.MailWebhook) service.MailWebhookRow {
	return service.MailWebhookRow{
		ID:        uint32(e.ID),
		Name:      e.Name,
		Address:   e.Address,
		URL:       e.URL,
		Secret:    e.Secret,
		Enabled:   e.Enabled,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

var _ service.MailWebhookStore = (*MailWebhookRepo)(nil)
