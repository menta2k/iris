// LoginPolicyRepo persists login-firewall rules (the login_policies table)
// and serves the enforcement read path (ListApplicable).
package data

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/loginpolicy"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

type LoginPolicyRepo struct{ client *ent.Client }

func NewLoginPolicyRepo(c *ent.Client) *LoginPolicyRepo {
	return &LoginPolicyRepo{client: c}
}

func (r *LoginPolicyRepo) List(ctx context.Context, limit, offset int) ([]service.LoginPolicyRow, error) {
	q := r.client.LoginPolicy.Query().
		Where(loginpolicy.DeletedAtIsNil()).
		Order(ent.Asc(loginpolicy.FieldID))
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("login_policy_repo: list: %w", err)
	}
	out := make([]service.LoginPolicyRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, loginPolicyToRow(e))
	}
	return out, nil
}

func (r *LoginPolicyRepo) Count(ctx context.Context) (int, error) {
	n, err := r.client.LoginPolicy.Query().
		Where(loginpolicy.DeletedAtIsNil()).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("login_policy_repo: count: %w", err)
	}
	return n, nil
}

func (r *LoginPolicyRepo) Get(ctx context.Context, id uint32) (*service.LoginPolicyRow, error) {
	e, err := r.client.LoginPolicy.Query().
		Where(loginpolicy.IDEQ(int(id)), loginpolicy.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("login_policy_repo: get: %w", err)
	}
	row := loginPolicyToRow(e)
	return &row, nil
}

func (r *LoginPolicyRepo) Create(ctx context.Context, in service.LoginPolicyRow) (*service.LoginPolicyRow, error) {
	saved, err := r.client.LoginPolicy.Create().
		SetTargetID(in.TargetID).
		SetType(loginpolicy.Type(in.Type)).
		SetMethod(loginpolicy.Method(in.Method)).
		SetValue(in.Value).
		SetTimeWindow(marshalTimeWindow(in.TimeWindow)).
		SetReason(in.Reason).
		SetEnabled(in.Enabled).
		SetCreatedBy(in.CreatedBy).
		SetUpdatedBy(in.UpdatedBy).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("login_policy_repo: create: %w", err)
	}
	row := loginPolicyToRow(saved)
	return &row, nil
}

func (r *LoginPolicyRepo) Update(ctx context.Context, id uint32, in service.LoginPolicyRow) (*service.LoginPolicyRow, error) {
	saved, err := r.client.LoginPolicy.UpdateOneID(int(id)).
		SetTargetID(in.TargetID).
		SetType(loginpolicy.Type(in.Type)).
		SetMethod(loginpolicy.Method(in.Method)).
		SetValue(in.Value).
		SetTimeWindow(marshalTimeWindow(in.TimeWindow)).
		SetReason(in.Reason).
		SetEnabled(in.Enabled).
		SetUpdatedBy(in.UpdatedBy).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("login_policy_repo: update: %w", err)
	}
	row := loginPolicyToRow(saved)
	return &row, nil
}

// Delete soft-deletes by stamping deleted_at/deleted_by. The row stays in
// the table but is hidden from List and excluded from ListApplicable.
func (r *LoginPolicyRepo) Delete(ctx context.Context, id, deletedBy uint32) error {
	err := r.client.LoginPolicy.UpdateOneID(int(id)).
		SetDeletedAt(time.Now()).
		SetDeletedBy(deletedBy).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("login_policy_repo: delete: %w", err)
	}
	return nil
}

// ListApplicable returns the enabled, non-deleted rules relevant to a login:
// global rules (target_id == 0) plus, when userID != nil, that user's rules.
func (r *LoginPolicyRepo) ListApplicable(ctx context.Context, userID *uint32) ([]service.LoginPolicyRow, error) {
	q := r.client.LoginPolicy.Query().Where(
		loginpolicy.EnabledEQ(true),
		loginpolicy.DeletedAtIsNil(),
	)
	if userID != nil {
		q = q.Where(loginpolicy.Or(
			loginpolicy.TargetIDEQ(0),
			loginpolicy.TargetIDEQ(*userID),
		))
	} else {
		q = q.Where(loginpolicy.TargetIDEQ(0))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("login_policy_repo: list_applicable: %w", err)
	}
	out := make([]service.LoginPolicyRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, loginPolicyToRow(e))
	}
	return out, nil
}

func loginPolicyToRow(e *ent.LoginPolicy) service.LoginPolicyRow {
	row := service.LoginPolicyRow{
		ID:         uint32(e.ID),
		TargetID:   e.TargetID,
		Type:       string(e.Type),
		Method:     string(e.Method),
		Value:      e.Value,
		TimeWindow: unmarshalTimeWindow(e.TimeWindow),
		Reason:     e.Reason,
		Enabled:    e.Enabled,
		CreatedBy:  e.CreatedBy,
		UpdatedBy:  e.UpdatedBy,
		DeletedBy:  e.DeletedBy,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}
	if e.DeletedAt != nil {
		t := *e.DeletedAt
		row.DeletedAt = &t
	}
	return row
}

// marshalTimeWindow encodes the structured window as JSON for the
// time_window string column. nil -> "".
func marshalTimeWindow(tw *service.TimeWindow) string {
	if tw == nil {
		return ""
	}
	b, err := json.Marshal(tw)
	if err != nil {
		return ""
	}
	return string(b)
}

func unmarshalTimeWindow(s string) *service.TimeWindow {
	if s == "" {
		return nil
	}
	var tw service.TimeWindow
	if err := json.Unmarshal([]byte(s), &tw); err != nil {
		return nil
	}
	return &tw
}

var (
	_ service.LoginPolicyStore = (*LoginPolicyRepo)(nil)
	_ service.RuleSource       = (*LoginPolicyRepo)(nil)
)
