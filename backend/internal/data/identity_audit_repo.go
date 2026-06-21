package data

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/menta2k/iris/backend/internal/biz"
)

// IdentityRepo persists Iris users and roles and resolves their permissions.
type IdentityRepo struct {
	db    *DB
	audit *AuditRepo
}

// NewIdentityRepo constructs the repository.
func NewIdentityRepo(db *DB, audit *AuditRepo) *IdentityRepo {
	return &IdentityRepo{db: db, audit: audit}
}

var _ biz.IdentityRepo = (*IdentityRepo)(nil)

// CreateUser inserts a user and assigns roles atomically.
func (r *IdentityRepo) CreateUser(ctx context.Context, u *biz.IrisUser) (*biz.IrisUser, error) {
	out := &biz.IrisUser{}
	err := r.db.InTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO iris_users (email, display_name, status, mfa_required)
			VALUES ($1, $2, $3, $4)
			RETURNING id, email, display_name, status, mfa_required`,
			u.Email, u.DisplayName, u.Status, u.MFARequired)
		if err := row.Scan(&out.ID, &out.Email, &out.DisplayName, &out.Status, &out.MFARequired); err != nil {
			return mapConstraint(err, "user")
		}
		for _, role := range u.Roles {
			if _, err := tx.Exec(ctx, `
				INSERT INTO user_roles (user_id, role_id)
				SELECT $1, id FROM roles WHERE name = $2`, out.ID, role); err != nil {
				return mapConstraint(err, "user_role")
			}
		}
		out.Roles = u.Roles
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListUsers returns users with their role names.
func (r *IdentityRepo) ListUsers(ctx context.Context, page biz.Page) ([]*biz.IrisUser, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT u.id, u.email, u.display_name, u.status, u.mfa_required,
		       coalesce(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), '{}')
		FROM iris_users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		GROUP BY u.id
		ORDER BY u.email
		LIMIT $1 OFFSET $2`, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()
	var out []*biz.IrisUser
	for rows.Next() {
		u := &biz.IrisUser{}
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Status, &u.MFARequired, &u.Roles); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// FindUserByEmail loads a single active user and its roles for authentication.
func (r *IdentityRepo) FindUserByEmail(ctx context.Context, email string) (*biz.IrisUser, error) {
	u := &biz.IrisUser{}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.display_name, u.status, u.mfa_required,
		       coalesce(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), '{}')
		FROM iris_users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		WHERE u.email = $1
		GROUP BY u.id`, email).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.Status, &u.MFARequired, &u.Roles)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, biz.NotFound("USER_NOT_FOUND", "user not found")
		}
		return nil, fmt.Errorf("find user: %w", err)
	}
	return u, nil
}

// SetUserStatus updates a user's status.
func (r *IdentityRepo) SetUserStatus(ctx context.Context, id, status string) error {
	tag, err := r.db.Pool.Exec(ctx,
		`UPDATE iris_users SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("update user status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("USER_NOT_FOUND", "user not found")
	}
	return nil
}

// GetMFA returns the user's TOTP secret and whether enrollment is confirmed
// (mfa_enrolled_at set). Satisfies biz.MFASecretStore.
func (r *IdentityRepo) GetMFA(ctx context.Context, userID string) (string, bool, error) {
	var secret string
	var enrolledAt *time.Time
	err := r.db.Pool.QueryRow(ctx,
		`SELECT mfa_secret, mfa_enrolled_at FROM iris_users WHERE id = $1`, userID).Scan(&secret, &enrolledAt)
	if err != nil {
		return "", false, fmt.Errorf("get mfa: %w", err)
	}
	return secret, enrolledAt != nil, nil
}

// SetMFASecret stores a pending (unconfirmed) secret, clearing enrollment.
func (r *IdentityRepo) SetMFASecret(ctx context.Context, userID, secret string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE iris_users SET mfa_secret = $2, mfa_enrolled_at = NULL, updated_at = now() WHERE id = $1`,
		userID, secret)
	if err != nil {
		return fmt.Errorf("set mfa secret: %w", err)
	}
	return nil
}

// MarkMFAEnrolled confirms enrollment by stamping mfa_enrolled_at.
func (r *IdentityRepo) MarkMFAEnrolled(ctx context.Context, userID string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE iris_users SET mfa_enrolled_at = now(), updated_at = now() WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("mark mfa enrolled: %w", err)
	}
	return nil
}

// ClearMFA removes a user's enrollment and secret.
func (r *IdentityRepo) ClearMFA(ctx context.Context, userID string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE iris_users SET mfa_secret = '', mfa_enrolled_at = NULL, updated_at = now() WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("clear mfa: %w", err)
	}
	return nil
}

// UpdateUser updates a user's profile, status, MFA requirement, and role set
// atomically by id.
func (r *IdentityRepo) UpdateUser(ctx context.Context, id string, u *biz.IrisUser) (*biz.IrisUser, error) {
	out := &biz.IrisUser{}
	err := r.db.InTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE iris_users SET display_name = $2, status = $3, mfa_required = $4, updated_at = now()
			WHERE id = $1
			RETURNING id, email, display_name, status, mfa_required`,
			id, u.DisplayName, u.Status, u.MFARequired)
		if err := row.Scan(&out.ID, &out.Email, &out.DisplayName, &out.Status, &out.MFARequired); err != nil {
			return mapConstraint(err, "user")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, id); err != nil {
			return mapConstraint(err, "user_role")
		}
		for _, role := range u.Roles {
			if _, err := tx.Exec(ctx, `
				INSERT INTO user_roles (user_id, role_id)
				SELECT $1, id FROM roles WHERE name = $2`, id, role); err != nil {
				return mapConstraint(err, "user_role")
			}
		}
		out.Roles = u.Roles
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListAuditEntries returns audit entries, delegating to the audit repository.
func (r *IdentityRepo) ListAuditEntries(ctx context.Context, page biz.Page) ([]*biz.AuditEntry, error) {
	items, err := r.audit.List(ctx, page)
	if err != nil {
		return nil, err
	}
	out := make([]*biz.AuditEntry, 0, len(items))
	for _, it := range items {
		out = append(out, &biz.AuditEntry{
			ID: it.ID, OccurredAt: it.OccurredAt, ActorUserID: it.ActorUserID,
			Operation: it.Operation, TargetType: it.TargetType, TargetID: it.TargetID,
			Outcome: it.Outcome, IPAddress: it.IPAddress, RequestID: it.RequestID,
			SafeChangeSummary: it.SafeChangeSummary,
		})
	}
	return out, nil
}
