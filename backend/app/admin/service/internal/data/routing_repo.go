// RoutingRepo backs service.RoutingStore with ent. Each rule has eager-loaded
// conditions and a single optional target.
package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/routingrule"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	"github.com/menta2k/iris/backend/pkg/kumopolicy"
)

// RoutingRepo persists routing rules.
type RoutingRepo struct{ client *ent.Client }

// NewRoutingRepo wires the ent client.
func NewRoutingRepo(c *ent.Client) *RoutingRepo { return &RoutingRepo{client: c} }

// List returns rules ordered by priority.
func (r *RoutingRepo) List(ctx context.Context, limit, offset int) ([]service.RoutingRow, uint32, error) {
	total, err := r.client.RoutingRule.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("routing_repo: count: %w", err)
	}
	rows, err := r.client.RoutingRule.Query().
		WithConditions().
		WithTarget().
		Order(ent.Asc(routingrule.FieldPriority)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("routing_repo: list: %w", err)
	}
	out := make([]service.RoutingRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, ruleToRow(e))
	}
	return out, uint32(total), nil
}

// Get one rule.
func (r *RoutingRepo) Get(ctx context.Context, id uint32) (*service.RoutingRow, error) {
	e, err := r.client.RoutingRule.Query().
		Where(routingrule.IDEQ(int(id))).
		WithConditions().
		WithTarget().
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("routing_repo: get: %w", err)
	}
	row := ruleToRow(e)
	return &row, nil
}

// Create inserts a rule plus its conditions and target in a single tx.
func (r *RoutingRepo) Create(ctx context.Context, in service.RoutingRow) (*service.RoutingRow, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("routing_repo: tx: %w", err)
	}
	rule, err := tx.RoutingRule.Create().
		SetName(in.Name).
		SetPriority(in.Priority).
		SetEnabled(in.Enabled).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("routing_repo: create rule: %w", err)
	}
	for _, c := range in.Conditions {
		if _, err := tx.RuleCondition.Create().
			SetField(c.Field).SetOp(c.Op).SetValue(c.Value).
			SetRuleID(rule.ID).
			Save(ctx); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("routing_repo: create condition: %w", err)
		}
	}
	if in.Target.Kind != "" {
		if _, err := tx.RuleTarget.Create().
			SetKind(in.Target.Kind).
			SetRef(in.Target.Ref).
			SetRejectCode(in.Target.RejectCode).
			SetRejectText(in.Target.RejectText).
			SetRuleID(rule.ID).
			Save(ctx); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("routing_repo: create target: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("routing_repo: commit: %w", err)
	}
	return r.Get(ctx, uint32(rule.ID))
}

// UpdateEnabled toggles the enabled flag.
func (r *RoutingRepo) UpdateEnabled(ctx context.Context, id uint32, enabled bool) (*service.RoutingRow, error) {
	if _, err := r.client.RoutingRule.UpdateOneID(int(id)).SetEnabled(enabled).Save(ctx); err != nil {
		return nil, fmt.Errorf("routing_repo: update enabled: %w", err)
	}
	return r.Get(ctx, id)
}

// Delete removes a rule (ent cascades the conditions and target via the
// edge definition's "Required" + DB-level FK ON DELETE CASCADE).
func (r *RoutingRepo) Delete(ctx context.Context, id uint32) error {
	if err := r.client.RoutingRule.DeleteOneID(int(id)).Exec(ctx); err != nil {
		return fmt.Errorf("routing_repo: delete: %w", err)
	}
	return nil
}

func ruleToRow(e *ent.RoutingRule) service.RoutingRow {
	conds := make([]kumopolicy.RuleCondition, 0, len(e.Edges.Conditions))
	for _, c := range e.Edges.Conditions {
		conds = append(conds, kumopolicy.RuleCondition{Field: c.Field, Op: c.Op, Value: c.Value})
	}
	var target kumopolicy.RuleTarget
	if e.Edges.Target != nil {
		target = kumopolicy.RuleTarget{
			Kind:       e.Edges.Target.Kind,
			Ref:        e.Edges.Target.Ref,
			RejectCode: e.Edges.Target.RejectCode,
			RejectText: e.Edges.Target.RejectText,
		}
	}
	return service.RoutingRow{
		ID:         uint32(e.ID),
		Name:       e.Name,
		Priority:   e.Priority,
		Enabled:    e.Enabled,
		Conditions: conds,
		Target:     target,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}
}

var _ service.RoutingStore = (*RoutingRepo)(nil)
