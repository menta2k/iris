// ListenerRepo backs service.ListenerStore with ent.
package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/listenerconfig"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// ListenerRepo persists kumomta SMTP listener rows.
type ListenerRepo struct{ client *ent.Client }

// NewListenerRepo wires the ent client.
func NewListenerRepo(c *ent.Client) *ListenerRepo { return &ListenerRepo{client: c} }

// List returns paginated listeners ordered by name (matching the order
// the renderer iterates them, so the rendered Lua block ordering tracks
// what the UI shows).
func (r *ListenerRepo) List(ctx context.Context, limit, offset int) ([]service.ListenerRow, uint32, error) {
	total, err := r.client.ListenerConfig.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("listener_repo: count: %w", err)
	}
	rows, err := r.client.ListenerConfig.Query().
		Order(ent.Asc(listenerconfig.FieldName)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("listener_repo: list: %w", err)
	}
	out := make([]service.ListenerRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, listenerToRow(e))
	}
	return out, uint32(total), nil
}

// Get one listener.
func (r *ListenerRepo) Get(ctx context.Context, id uint32) (*service.ListenerRow, error) {
	e, err := r.client.ListenerConfig.Get(ctx, int(id))
	if err != nil {
		return nil, fmt.Errorf("listener_repo: get: %w", err)
	}
	row := listenerToRow(e)
	return &row, nil
}

// Create inserts a new listener.
func (r *ListenerRepo) Create(ctx context.Context, in service.ListenerRow) (*service.ListenerRow, error) {
	created, err := r.client.ListenerConfig.Create().
		SetName(in.Name).
		SetListenAddr(in.ListenAddr).
		SetHostname(in.Hostname).
		SetTLSEnabled(in.TLSEnabled).
		SetTLSCertPemPath(in.TLSCertPath).
		SetTLSKeyPemPath(in.TLSKeyPath).
		SetRequireAuth(in.RequireAuth).
		SetMaxMessageSize(in.MaxMessageSize).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("listener_repo: create: %w", err)
	}
	out := listenerToRow(created)
	return &out, nil
}

// Update mutates an existing listener. Name is intentionally not
// updatable here — service-layer validate() rejects rename attempts so
// the UI flow is "delete and recreate" (same as VMTAs).
func (r *ListenerRepo) Update(ctx context.Context, id uint32, in service.ListenerRow) (*service.ListenerRow, error) {
	updated, err := r.client.ListenerConfig.UpdateOneID(int(id)).
		SetListenAddr(in.ListenAddr).
		SetHostname(in.Hostname).
		SetTLSEnabled(in.TLSEnabled).
		SetTLSCertPemPath(in.TLSCertPath).
		SetTLSKeyPemPath(in.TLSKeyPath).
		SetRequireAuth(in.RequireAuth).
		SetMaxMessageSize(in.MaxMessageSize).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("listener_repo: update: %w", err)
	}
	out := listenerToRow(updated)
	return &out, nil
}

// Delete removes a listener. Domain edges cascade via ent's ondelete
// default; if you've added explicit domain rows on the listener-domains
// page they'll go with the parent.
func (r *ListenerRepo) Delete(ctx context.Context, id uint32) error {
	if err := r.client.ListenerConfig.DeleteOneID(int(id)).Exec(ctx); err != nil {
		return fmt.Errorf("listener_repo: delete: %w", err)
	}
	return nil
}

func listenerToRow(e *ent.ListenerConfig) service.ListenerRow {
	return service.ListenerRow{
		ID:             uint32(e.ID),
		Name:           e.Name,
		ListenAddr:     e.ListenAddr,
		Hostname:       e.Hostname,
		TLSEnabled:     e.TLSEnabled,
		TLSCertPath:    e.TLSCertPemPath,
		TLSKeyPath:     e.TLSKeyPemPath,
		RequireAuth:    e.RequireAuth,
		MaxMessageSize: e.MaxMessageSize,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}

// Compile-time check.
var _ service.ListenerStore = (*ListenerRepo)(nil)
