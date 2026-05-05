// PolicyHistoryRepo backs service.PolicyHistoryWriter with ent.
package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/policyhistory"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// PolicyHistoryRepo persists/reads policy apply history.
type PolicyHistoryRepo struct{ client *ent.Client }

// NewPolicyHistoryRepo wires the ent client.
func NewPolicyHistoryRepo(c *ent.Client) *PolicyHistoryRepo { return &PolicyHistoryRepo{client: c} }

// Append records a successful apply.
func (r *PolicyHistoryRepo) Append(ctx context.Context, sha256Hex, note, luaSource string, actorUserID uint32) error {
	_, err := r.client.PolicyHistory.Create().
		SetSha256(sha256Hex).
		SetNote(note).
		SetLuaSource(luaSource).
		SetActorUserID(actorUserID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("policy_history: append: %w", err)
	}
	return nil
}

// List returns the most recent N records.
func (r *PolicyHistoryRepo) List(ctx context.Context, limit int) ([]service.PolicyHistoryRow, error) {
	rows, err := r.client.PolicyHistory.Query().
		Order(ent.Desc(policyhistory.FieldAppliedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("policy_history: list: %w", err)
	}
	out := make([]service.PolicyHistoryRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, service.PolicyHistoryRow{
			ID:          uint64(e.ID),
			SHA256:      e.Sha256,
			Note:        e.Note,
			ActorUserID: e.ActorUserID,
			AppliedAt:   e.AppliedAt,
		})
	}
	return out, nil
}

var _ service.PolicyHistoryWriter = (*PolicyHistoryRepo)(nil)
