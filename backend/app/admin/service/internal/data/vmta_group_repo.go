// VmtaGroupRepo backs service.VmtaGroupStore with ent.
package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/virtualmtagroup"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/virtualmtagroupmember"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// VmtaGroupRepo persists VMTA groups + their weighted member edges.
type VmtaGroupRepo struct{ client *ent.Client }

// NewVmtaGroupRepo wires the ent client.
func NewVmtaGroupRepo(c *ent.Client) *VmtaGroupRepo { return &VmtaGroupRepo{client: c} }

// List paginates groups (no member preload — call Get for member detail).
func (r *VmtaGroupRepo) List(ctx context.Context, limit, offset int) ([]service.VmtaGroupRow, uint32, error) {
	total, err := r.client.VirtualMtaGroup.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("vmta_group_repo: count: %w", err)
	}
	rows, err := r.client.VirtualMtaGroup.Query().
		Order(ent.Asc(virtualmtagroup.FieldName)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("vmta_group_repo: list: %w", err)
	}
	out := make([]service.VmtaGroupRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, vmtaGroupToRow(e, nil))
	}
	return out, uint32(total), nil
}

// Get returns the group with members (and member.vmta) eagerly loaded.
func (r *VmtaGroupRepo) Get(ctx context.Context, id uint32) (*service.VmtaGroupRow, error) {
	g, err := r.client.VirtualMtaGroup.Query().
		Where(virtualmtagroup.IDEQ(int(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("vmta_group_repo: get: %w", err)
	}
	members, err := r.loadMembers(ctx, id)
	if err != nil {
		return nil, err
	}
	row := vmtaGroupToRow(g, members)
	return &row, nil
}

// Create persists a new group. Members must be added with SetMembers.
func (r *VmtaGroupRepo) Create(ctx context.Context, in service.VmtaGroupRow) (*service.VmtaGroupRow, error) {
	g, err := r.client.VirtualMtaGroup.Create().
		SetName(in.Name).
		SetDescription(in.Description).
		SetEnabled(in.Enabled).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("vmta_group_repo: create: %w", err)
	}
	row := vmtaGroupToRow(g, nil)
	return &row, nil
}

// Update overwrites mutable fields.
func (r *VmtaGroupRepo) Update(ctx context.Context, id uint32, in service.VmtaGroupRow) (*service.VmtaGroupRow, error) {
	g, err := r.client.VirtualMtaGroup.UpdateOneID(int(id)).
		SetName(in.Name).
		SetDescription(in.Description).
		SetEnabled(in.Enabled).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("vmta_group_repo: update: %w", err)
	}
	members, err := r.loadMembers(ctx, id)
	if err != nil {
		return nil, err
	}
	row := vmtaGroupToRow(g, members)
	return &row, nil
}

// Delete removes a group plus its membership edges.
func (r *VmtaGroupRepo) Delete(ctx context.Context, id uint32) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("vmta_group_repo: tx: %w", err)
	}
	if _, err := tx.VirtualMtaGroupMember.Delete().
		Where(virtualmtagroupmember.HasGroupWith(virtualmtagroup.IDEQ(int(id)))).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("vmta_group_repo: delete members: %w", err)
	}
	if err := tx.VirtualMtaGroup.DeleteOneID(int(id)).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("vmta_group_repo: delete group: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("vmta_group_repo: commit: %w", err)
	}
	return nil
}

// SetMembers replaces the group's membership atomically: drop the existing
// membership edges, then re-create from the input. Wrapped in a tx.
func (r *VmtaGroupRepo) SetMembers(ctx context.Context, groupID uint32, members []service.VmtaGroupMemberRow) (*service.VmtaGroupRow, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("vmta_group_repo: tx: %w", err)
	}
	if _, err := tx.VirtualMtaGroupMember.Delete().
		Where(virtualmtagroupmember.HasGroupWith(virtualmtagroup.IDEQ(int(groupID)))).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("vmta_group_repo: clear members: %w", err)
	}
	for _, m := range members {
		if _, err := tx.VirtualMtaGroupMember.Create().
			SetGroupID(int(groupID)).
			SetVmtaID(int(m.VmtaID)).
			SetWeight(m.Weight).
			SetPriority(m.Priority).
			SetEnabled(m.Enabled).
			Save(ctx); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("vmta_group_repo: add member vmta_id=%d: %w", m.VmtaID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("vmta_group_repo: commit: %w", err)
	}
	return r.Get(ctx, groupID)
}

func (r *VmtaGroupRepo) loadMembers(ctx context.Context, groupID uint32) ([]service.VmtaGroupMemberRow, error) {
	rows, err := r.client.VirtualMtaGroupMember.Query().
		Where(virtualmtagroupmember.HasGroupWith(virtualmtagroup.IDEQ(int(groupID)))).
		WithVmta().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("vmta_group_repo: members: %w", err)
	}
	out := make([]service.VmtaGroupMemberRow, 0, len(rows))
	for _, m := range rows {
		row := service.VmtaGroupMemberRow{
			Weight:   m.Weight,
			Priority: m.Priority,
			Enabled:  m.Enabled,
		}
		if v, _ := m.Edges.VmtaOrErr(); v != nil {
			row.VmtaID = uint32(v.ID)
			row.VmtaName = v.Name
		}
		out = append(out, row)
	}
	return out, nil
}

func vmtaGroupToRow(g *ent.VirtualMtaGroup, members []service.VmtaGroupMemberRow) service.VmtaGroupRow {
	return service.VmtaGroupRow{
		ID:          uint32(g.ID),
		Name:        g.Name,
		Description: g.Description,
		Enabled:     g.Enabled,
		Members:     members,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
}

var _ service.VmtaGroupStore = (*VmtaGroupRepo)(nil)
