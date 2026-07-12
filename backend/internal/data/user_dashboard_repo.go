package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/menta2k/iris/backend/internal/biz"
)

// UserDashboardRepo persists per-user custom dashboards. Every query is scoped by
// user_id so cross-user access is impossible at the SQL layer.
type UserDashboardRepo struct {
	db *DB
}

// NewUserDashboardRepo constructs the repository.
func NewUserDashboardRepo(db *DB) *UserDashboardRepo { return &UserDashboardRepo{db: db} }

var _ biz.UserDashboardRepo = (*UserDashboardRepo)(nil)

const userDashboardCols = `id, user_id, name, is_default, widgets, created_at, updated_at`

func scanUserDashboard(row pgx.Row) (*biz.UserDashboard, error) {
	d := &biz.UserDashboard{}
	if err := row.Scan(&d.ID, &d.UserID, &d.Name, &d.IsDefault, &d.Widgets, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, err
	}
	return d, nil
}

// List returns the user's dashboards, oldest first.
func (r *UserDashboardRepo) List(ctx context.Context, userID string) ([]*biz.UserDashboard, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+userDashboardCols+` FROM user_dashboards WHERE user_id = $1 ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user dashboards: %w", err)
	}
	defer rows.Close()
	var out []*biz.UserDashboard
	for rows.Next() {
		d, err := scanUserDashboard(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user dashboard: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Get returns one of the user's dashboards, or a NotFound domain error.
func (r *UserDashboardRepo) Get(ctx context.Context, userID, id string) (*biz.UserDashboard, error) {
	d, err := scanUserDashboard(r.db.Pool.QueryRow(ctx,
		`SELECT `+userDashboardCols+` FROM user_dashboards WHERE user_id = $1 AND id = $2`, userID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.NotFound("DASHBOARD_NOT_FOUND", "dashboard %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get user dashboard: %w", err)
	}
	return d, nil
}

// Create inserts a dashboard.
func (r *UserDashboardRepo) Create(ctx context.Context, d *biz.UserDashboard) (*biz.UserDashboard, error) {
	out, err := scanUserDashboard(r.db.Pool.QueryRow(ctx, `
		INSERT INTO user_dashboards (user_id, name, is_default, widgets)
		VALUES ($1, $2, $3, $4)
		RETURNING `+userDashboardCols,
		d.UserID, d.Name, d.IsDefault, d.Widgets))
	if err != nil {
		return nil, mapConstraint(err, "user_dashboard")
	}
	return out, nil
}

// Update edits a dashboard's name + widgets (not is_default — see SetDefault).
func (r *UserDashboardRepo) Update(ctx context.Context, d *biz.UserDashboard) (*biz.UserDashboard, error) {
	out, err := scanUserDashboard(r.db.Pool.QueryRow(ctx, `
		UPDATE user_dashboards SET name = $3, widgets = $4, updated_at = now()
		WHERE user_id = $1 AND id = $2
		RETURNING `+userDashboardCols,
		d.UserID, d.ID, d.Name, d.Widgets))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.NotFound("DASHBOARD_NOT_FOUND", "dashboard %q not found", d.ID)
	}
	if err != nil {
		return nil, mapConstraint(err, "user_dashboard")
	}
	return out, nil
}

// Delete removes a dashboard.
func (r *UserDashboardRepo) Delete(ctx context.Context, userID, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM user_dashboards WHERE user_id = $1 AND id = $2`, userID, id)
	if err != nil {
		return fmt.Errorf("delete user dashboard: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("DASHBOARD_NOT_FOUND", "dashboard %q not found", id)
	}
	return nil
}

// SetDefault clears the user's prior default and marks id as default, in one
// transaction so the partial-unique index is never transiently violated.
func (r *UserDashboardRepo) SetDefault(ctx context.Context, userID, id string) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin set-default: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE user_dashboards SET is_default = false WHERE user_id = $1 AND is_default`, userID); err != nil {
		return fmt.Errorf("clear default: %w", err)
	}
	tag, err := tx.Exec(ctx,
		`UPDATE user_dashboards SET is_default = true, updated_at = now() WHERE user_id = $1 AND id = $2`, userID, id)
	if err != nil {
		return fmt.Errorf("set default: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("DASHBOARD_NOT_FOUND", "dashboard %q not found", id)
	}
	return tx.Commit(ctx)
}
