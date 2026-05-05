package service

import (
	"context"
	"errors"
	"time"

	"github.com/menta2k/iris/backend/pkg/kumopolicy"
)

// RoutingRow is the rule together with its conditions and target. The
// service-layer representation mirrors kumopolicy.RoutingRule so the
// snapshot loader is a flat copy.
//
// JSON tags are defined so the type can serve double-duty as the HTTP
// create-request body — the registrar decodes directly into this struct.
type RoutingRow struct {
	ID         uint32                     `json:"id,omitempty"`
	Name       string                     `json:"name"`
	Priority   int32                      `json:"priority"`
	Enabled    bool                       `json:"enabled"`
	Conditions []kumopolicy.RuleCondition `json:"conditions"`
	Target     kumopolicy.RuleTarget      `json:"target"`
	CreatedAt  time.Time                  `json:"created_at,omitempty"`
	UpdatedAt  time.Time                  `json:"updated_at,omitempty"`
}

// RoutingStore is the data-layer interface.
type RoutingStore interface {
	List(ctx context.Context, limit, offset int) ([]RoutingRow, uint32, error)
	Get(ctx context.Context, id uint32) (*RoutingRow, error)
	Create(ctx context.Context, in RoutingRow) (*RoutingRow, error)
	UpdateEnabled(ctx context.Context, id uint32, enabled bool) (*RoutingRow, error)
	Delete(ctx context.Context, id uint32) error
}

// RoutingService implements rule CRUD. Validation is delegated to
// kumopolicy.Snapshot.Validate at apply time — at the row level we only
// reject obvious shape errors so the UI can still save in-progress rules.
type RoutingService struct{ store RoutingStore }

// NewRoutingService constructs the service.
func NewRoutingService(store RoutingStore) *RoutingService {
	return &RoutingService{store: store}
}

// List paginates rules by priority ascending.
func (s *RoutingService) List(ctx context.Context, limit, offset int) ([]RoutingRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.List(ctx, limit, offset)
}

// Get one rule.
func (s *RoutingService) Get(ctx context.Context, id uint32) (*RoutingRow, error) {
	if id == 0 {
		return nil, errors.New("routing: id required")
	}
	return s.store.Get(ctx, id)
}

// Create inserts a new rule. Server-side validation:
//   - name must be non-empty
//   - target.kind must be in kumopolicy.AllowedTargetKinds
//   - every condition.field must be in kumopolicy.AllowedConditionFields
func (s *RoutingService) Create(ctx context.Context, in *RoutingRow) (*RoutingRow, error) {
	if in == nil || in.Name == "" {
		return nil, errors.New("routing: name required")
	}
	if _, ok := kumopolicy.AllowedTargetKinds[in.Target.Kind]; !ok {
		return nil, errors.New("routing: target.kind invalid")
	}
	for _, c := range in.Conditions {
		if _, ok := kumopolicy.AllowedConditionFields[c.Field]; !ok {
			return nil, errors.New("routing: condition.field invalid")
		}
	}
	return s.store.Create(ctx, *in)
}

// UpdateEnabled flips the enabled flag.
func (s *RoutingService) UpdateEnabled(ctx context.Context, id uint32, enabled bool) (*RoutingRow, error) {
	if id == 0 {
		return nil, errors.New("routing: id required")
	}
	return s.store.UpdateEnabled(ctx, id, enabled)
}

// Delete removes by id.
func (s *RoutingService) Delete(ctx context.Context, id uint32) error {
	if id == 0 {
		return errors.New("routing: id required")
	}
	return s.store.Delete(ctx, id)
}
