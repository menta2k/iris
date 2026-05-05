// VmtaRepo backs service.VirtualMtaStore with ent.
package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/virtualmta"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// VmtaRepo persists VMTAs.
type VmtaRepo struct{ client *ent.Client }

// NewVmtaRepo wires the ent client.
func NewVmtaRepo(c *ent.Client) *VmtaRepo { return &VmtaRepo{client: c} }

// List returns paginated VMTAs ordered by name.
func (r *VmtaRepo) List(ctx context.Context, limit, offset int) ([]service.VirtualMtaRow, uint32, error) {
	total, err := r.client.VirtualMta.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("vmta_repo: count: %w", err)
	}
	rows, err := r.client.VirtualMta.Query().
		Order(ent.Asc(virtualmta.FieldName)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("vmta_repo: list: %w", err)
	}
	out := make([]service.VirtualMtaRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, vmtaToRow(e))
	}
	return out, uint32(total), nil
}

// Get one VMTA.
func (r *VmtaRepo) Get(ctx context.Context, id uint32) (*service.VirtualMtaRow, error) {
	e, err := r.client.VirtualMta.Get(ctx, int(id))
	if err != nil {
		return nil, fmt.Errorf("vmta_repo: get: %w", err)
	}
	row := vmtaToRow(e)
	return &row, nil
}

// Create inserts a new VMTA.
func (r *VmtaRepo) Create(ctx context.Context, in service.VirtualMtaRow) (*service.VirtualMtaRow, error) {
	created, err := r.client.VirtualMta.Create().
		SetName(in.Name).
		SetSourceIps(strings.Join(in.SourceIPs, ",")).
		SetHeloName(in.HeloName).
		SetMaxConnections(in.MaxConnections).
		SetMaxMessagesPerConnection(in.MaxMessagesPerConnection).
		SetConnectTimeout(in.ConnectTimeout).
		SetProviderProfile(nonEmpty(in.ProviderProfile, "default")).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("vmta_repo: create: %w", err)
	}
	out := vmtaToRow(created)
	return &out, nil
}

// Update mutates an existing VMTA.
func (r *VmtaRepo) Update(ctx context.Context, id uint32, in service.VirtualMtaRow) (*service.VirtualMtaRow, error) {
	updated, err := r.client.VirtualMta.UpdateOneID(int(id)).
		SetSourceIps(strings.Join(in.SourceIPs, ",")).
		SetHeloName(in.HeloName).
		SetMaxConnections(in.MaxConnections).
		SetMaxMessagesPerConnection(in.MaxMessagesPerConnection).
		SetConnectTimeout(in.ConnectTimeout).
		SetProviderProfile(nonEmpty(in.ProviderProfile, "default")).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("vmta_repo: update: %w", err)
	}
	out := vmtaToRow(updated)
	return &out, nil
}

// Delete removes a VMTA.
func (r *VmtaRepo) Delete(ctx context.Context, id uint32) error {
	if err := r.client.VirtualMta.DeleteOneID(int(id)).Exec(ctx); err != nil {
		return fmt.Errorf("vmta_repo: delete: %w", err)
	}
	return nil
}

func vmtaToRow(e *ent.VirtualMta) service.VirtualMtaRow {
	ips := []string{}
	if e.SourceIps != "" {
		for _, p := range strings.Split(e.SourceIps, ",") {
			if t := strings.TrimSpace(p); t != "" {
				ips = append(ips, t)
			}
		}
	}
	return service.VirtualMtaRow{
		ID:                       uint32(e.ID),
		Name:                     e.Name,
		SourceIPs:                ips,
		HeloName:                 e.HeloName,
		MaxConnections:           e.MaxConnections,
		MaxMessagesPerConnection: e.MaxMessagesPerConnection,
		ConnectTimeout:           e.ConnectTimeout,
		ProviderProfile:          e.ProviderProfile,
		CreatedAt:                e.CreatedAt,
		UpdatedAt:                e.UpdatedAt,
	}
}

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

var _ service.VirtualMtaStore = (*VmtaRepo)(nil)
