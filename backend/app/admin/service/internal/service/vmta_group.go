package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// VmtaGroupRow is the data-layer view of a VMTA group plus its (optional)
// resolved member list. List endpoints return groups without members; Get
// returns the group with members eagerly loaded.
type VmtaGroupRow struct {
	ID          uint32              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Enabled     bool                `json:"enabled"`
	Members     []VmtaGroupMemberRow `json:"members,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// VmtaGroupMemberRow associates one VirtualMta with a group + weight.
// VmtaName is denormalised onto the row for display; the source of truth
// is the (group_id, vmta_id) edge.
type VmtaGroupMemberRow struct {
	VmtaID   uint32 `json:"vmta_id"`
	VmtaName string `json:"vmta_name,omitempty"`
	Weight   uint32 `json:"weight"`
	Priority uint32 `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

// VmtaGroupStore is the data-layer interface implemented by the ent repo.
type VmtaGroupStore interface {
	List(ctx context.Context, limit, offset int) ([]VmtaGroupRow, uint32, error)
	Get(ctx context.Context, id uint32) (*VmtaGroupRow, error)
	Create(ctx context.Context, in VmtaGroupRow) (*VmtaGroupRow, error)
	Update(ctx context.Context, id uint32, in VmtaGroupRow) (*VmtaGroupRow, error)
	Delete(ctx context.Context, id uint32) error

	// SetMembers replaces the group's membership atomically: rows present
	// in the input are upserted (group_id, vmta_id is unique), and rows
	// absent from the input are deleted. Idempotent.
	SetMembers(ctx context.Context, groupID uint32, members []VmtaGroupMemberRow) (*VmtaGroupRow, error)
}

// VmtaGroupService validates inputs and delegates persistence. The service
// is the source of truth for naming rules; the repo enforces uniqueness.
type VmtaGroupService struct {
	store VmtaGroupStore
	now   func() time.Time
}

// NewVmtaGroupService constructs the service.
func NewVmtaGroupService(store VmtaGroupStore) *VmtaGroupService {
	return &VmtaGroupService{store: store, now: time.Now}
}

// reVmtaGroupName uses the same safe-name set as routing target refs so a
// rule's target.ref can refer to a group by name.
var reVmtaGroupName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// Errors
var (
	ErrVmtaGroupName = errors.New("vmta_group: name must match [A-Za-z0-9_.-]{1,64}")
	ErrVmtaGroupID   = errors.New("vmta_group: id required")
)

// CreateVmtaGroupInput is the validated input shape for Create/Update.
type CreateVmtaGroupInput struct {
	Name        string
	Description string
	Enabled     bool
}

// List paginates groups (without members; call Get for member detail).
func (s *VmtaGroupService) List(ctx context.Context, limit, offset int) ([]VmtaGroupRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.List(ctx, limit, offset)
}

// Get returns the group with members eagerly loaded.
func (s *VmtaGroupService) Get(ctx context.Context, id uint32) (*VmtaGroupRow, error) {
	if id == 0 {
		return nil, ErrVmtaGroupID
	}
	return s.store.Get(ctx, id)
}

// Create persists a new group. Members are attached via SetMembers in a
// follow-up call so the wire shape mirrors REST conventions.
func (s *VmtaGroupService) Create(ctx context.Context, in *CreateVmtaGroupInput) (*VmtaGroupRow, error) {
	if in == nil {
		return nil, ErrVmtaGroupName
	}
	name := strings.TrimSpace(in.Name)
	if !reVmtaGroupName.MatchString(name) {
		return nil, ErrVmtaGroupName
	}
	row := VmtaGroupRow{
		Name:        name,
		Description: clipString(in.Description, 512),
		Enabled:     in.Enabled,
		CreatedAt:   s.now().UTC(),
		UpdatedAt:   s.now().UTC(),
	}
	out, err := s.store.Create(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("vmta_group: create: %w", err)
	}
	return out, nil
}

// Update overwrites the mutable fields on an existing group. Membership is
// edited separately via SetMembers.
func (s *VmtaGroupService) Update(ctx context.Context, id uint32, in *CreateVmtaGroupInput) (*VmtaGroupRow, error) {
	if id == 0 {
		return nil, ErrVmtaGroupID
	}
	if in == nil {
		return nil, ErrVmtaGroupName
	}
	name := strings.TrimSpace(in.Name)
	if !reVmtaGroupName.MatchString(name) {
		return nil, ErrVmtaGroupName
	}
	row := VmtaGroupRow{
		Name:        name,
		Description: clipString(in.Description, 512),
		Enabled:     in.Enabled,
		UpdatedAt:   s.now().UTC(),
	}
	out, err := s.store.Update(ctx, id, row)
	if err != nil {
		return nil, fmt.Errorf("vmta_group: update: %w", err)
	}
	return out, nil
}

// Delete removes a group and all its membership edges (cascade is enforced
// at the repo layer). Routing rules referencing the deleted group by name
// will fail validation at apply time until edited.
func (s *VmtaGroupService) Delete(ctx context.Context, id uint32) error {
	if id == 0 {
		return ErrVmtaGroupID
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return fmt.Errorf("vmta_group: delete: %w", err)
	}
	return nil
}

// SetMembers replaces the group's membership atomically. Members with weight
// 0 are accepted (they remain in the group but are excluded from the random
// draw — useful for soft-disabling without losing per-member config).
func (s *VmtaGroupService) SetMembers(ctx context.Context, groupID uint32, members []VmtaGroupMemberRow) (*VmtaGroupRow, error) {
	if groupID == 0 {
		return nil, ErrVmtaGroupID
	}
	// Defensive copy + dedupe by VmtaID; a duplicate would violate the
	// unique index in the repo, so we surface a clean error here.
	seen := make(map[uint32]struct{}, len(members))
	clean := make([]VmtaGroupMemberRow, 0, len(members))
	for _, m := range members {
		if m.VmtaID == 0 {
			return nil, errors.New("vmta_group: member.vmta_id required")
		}
		if _, dup := seen[m.VmtaID]; dup {
			return nil, fmt.Errorf("vmta_group: duplicate member vmta_id=%d", m.VmtaID)
		}
		seen[m.VmtaID] = struct{}{}
		clean = append(clean, m)
	}
	out, err := s.store.SetMembers(ctx, groupID, clean)
	if err != nil {
		return nil, fmt.Errorf("vmta_group: set members: %w", err)
	}
	return out, nil
}
