// UserRepo is the ent-backed implementation of service.UserStore + adapter
// helpers used by the authentication service.
package data

import (
	"context"
	"fmt"
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/role"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/user"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// UserRepo persists and queries users.
type UserRepo struct {
	client *ent.Client
}

// NewUserRepo wires the ent client into the repo.
func NewUserRepo(c *ent.Client) *UserRepo { return &UserRepo{client: c} }

// FindByUsername returns the user row for service-layer auth checks. The
// returned slice of roles is the role *codes* (not IDs), to match the
// service.UserRow contract and JWT claims shape.
func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*service.UserRow, error) {
	u, err := r.client.User.Query().
		Where(user.UsernameEQ(username)).
		WithRoles().
		Only(ctx)
	if err != nil {
		return nil, err
	}
	roles := make([]string, 0, len(u.Edges.Roles))
	for _, role := range u.Edges.Roles {
		roles = append(roles, role.Code)
	}
	return &service.UserRow{
		ID:           uint32(u.ID),
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		Active:       u.Active,
		Roles:        roles,
	}, nil
}

// RecordLoginSuccess updates last_login_{at,ip} and clears failed_logins.
func (r *UserRepo) RecordLoginSuccess(ctx context.Context, userID uint32, ip string, at time.Time) error {
	_, err := r.client.User.UpdateOneID(int(userID)).
		SetLastLoginAt(at).
		SetLastLoginIP(ip).
		SetFailedLogins(0).
		ClearLockedUntil().
		Save(ctx)
	if err != nil {
		return fmt.Errorf("user_repo: record success: %w", err)
	}
	return nil
}

// RecordLoginFailure increments failed_logins for the supplied username if
// it exists. The query is by-username (not by-id) so failures for unknown
// usernames simply no-op without leaking user existence to the caller.
func (r *UserRepo) RecordLoginFailure(ctx context.Context, username string, at time.Time) error {
	u, err := r.client.User.Query().
		Where(user.UsernameEQ(username)).
		Only(ctx)
	if err != nil {
		// Either not found or genuine error — both are non-fatal for the
		// caller (Login already returns ErrInvalidCredentials).
		return nil
	}
	const lockoutThreshold = 5
	const lockoutDuration = 15 * time.Minute

	upd := r.client.User.UpdateOneID(u.ID).
		SetFailedLogins(u.FailedLogins + 1)
	if u.FailedLogins+1 >= lockoutThreshold {
		upd = upd.SetLockedUntil(at.Add(lockoutDuration))
	}
	_, err = upd.Save(ctx)
	if err != nil {
		return fmt.Errorf("user_repo: record failure: %w", err)
	}
	return nil
}

// IsLockedOut returns true iff locked_until is in the future.
func (r *UserRepo) IsLockedOut(ctx context.Context, userID uint32, at time.Time) (bool, error) {
	u, err := r.client.User.Get(ctx, int(userID))
	if err != nil {
		return false, err
	}
	if u.LockedUntil == nil {
		return false, nil
	}
	return u.LockedUntil.After(at), nil
}

// --- Admin CRUD (service.UserAdminStore) -----------------------------------

// List returns active+inactive users ordered by id, with pagination.
// Total count is the unfiltered table size.
func (r *UserRepo) List(ctx context.Context, limit, offset int) ([]service.UserRow, uint32, error) {
	total, err := r.client.User.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("user_repo: count: %w", err)
	}
	rows, err := r.client.User.Query().
		WithRoles().
		Order(ent.Asc(user.FieldID)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("user_repo: list: %w", err)
	}
	out := make([]service.UserRow, 0, len(rows))
	for _, u := range rows {
		out = append(out, entUserToRow(u))
	}
	return out, uint32(total), nil
}

// Get fetches a user by id with roles eager-loaded.
func (r *UserRepo) Get(ctx context.Context, id uint32) (*service.UserRow, error) {
	u, err := r.client.User.Query().
		Where(user.IDEQ(int(id))).
		WithRoles().
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("user_repo: get: %w", err)
	}
	row := entUserToRow(u)
	return &row, nil
}

// Create inserts a user, attaching role edges by code. Roles missing from the
// roles table are silently ignored — caller is expected to validate.
func (r *UserRepo) Create(ctx context.Context, in service.UserCreateInput) (*service.UserRow, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("user_repo: tx: %w", err)
	}
	rolesIDs, err := roleIDsByCode(ctx, tx.Client(), in.Roles)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	created, err := tx.User.Create().
		SetUsername(in.Username).
		SetEmail(in.Email).
		SetDisplayName(in.DisplayName).
		SetPasswordHash(in.PasswordHash).
		SetActive(in.Active).
		AddRoleIDs(rolesIDs...).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("user_repo: create: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("user_repo: commit: %w", err)
	}
	full, err := r.Get(ctx, uint32(created.ID))
	if err != nil {
		return nil, err
	}
	return full, nil
}

// Update mutates only the supplied fields. Role replacement is all-or-nothing
// — when in.Roles is non-nil the user's role edges are replaced wholesale.
func (r *UserRepo) Update(ctx context.Context, id uint32, in service.UserUpdateInput) (*service.UserRow, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("user_repo: tx: %w", err)
	}
	upd := tx.User.UpdateOneID(int(id))
	if in.Email != nil {
		upd = upd.SetEmail(*in.Email)
	}
	if in.DisplayName != nil {
		upd = upd.SetDisplayName(*in.DisplayName)
	}
	if in.Active != nil {
		upd = upd.SetActive(*in.Active)
		if !*in.Active {
			upd = upd.SetDeactivatedAt(time.Now())
		} else {
			upd = upd.ClearDeactivatedAt()
		}
	}
	if in.Roles != nil {
		ids, err := roleIDsByCode(ctx, tx.Client(), *in.Roles)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		upd = upd.ClearRoles().AddRoleIDs(ids...)
	}
	if _, err := upd.Save(ctx); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("user_repo: update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("user_repo: commit: %w", err)
	}
	return r.Get(ctx, id)
}

// Delete is a soft-delete: active=false and deactivated_at=now. Hard delete
// would orphan audit_entry.actor_user_id rows.
func (r *UserRepo) Delete(ctx context.Context, id uint32) error {
	_, err := r.client.User.UpdateOneID(int(id)).
		SetActive(false).
		SetDeactivatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("user_repo: soft-delete: %w", err)
	}
	return nil
}

// UpdatePassword stores a new bcrypt hash. Caller is responsible for hashing
// (the service layer enforces the password policy first).
func (r *UserRepo) UpdatePassword(ctx context.Context, id uint32, hash string) error {
	_, err := r.client.User.UpdateOneID(int(id)).
		SetPasswordHash(hash).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("user_repo: update password: %w", err)
	}
	return nil
}

// GetPasswordHash returns the bcrypt hash for self-service password change.
func (r *UserRepo) GetPasswordHash(ctx context.Context, id uint32) (string, error) {
	u, err := r.client.User.Query().
		Where(user.IDEQ(int(id))).
		Select(user.FieldPasswordHash).
		Only(ctx)
	if err != nil {
		return "", fmt.Errorf("user_repo: get password hash: %w", err)
	}
	return u.PasswordHash, nil
}

// roleIDsByCode resolves role codes to ent IDs. Unknown codes are skipped
// (not errored) so the caller can choose whether to validate. Empty input
// returns a nil slice.
func roleIDsByCode(ctx context.Context, client *ent.Client, codes []string) ([]int, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	rs, err := client.Role.Query().Where(role.CodeIn(codes...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("user_repo: lookup roles: %w", err)
	}
	out := make([]int, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out, nil
}

// entUserToRow flattens the ent.User (+ eager-loaded roles) to service.UserRow.
func entUserToRow(u *ent.User) service.UserRow {
	roles := make([]string, 0, len(u.Edges.Roles))
	for _, r := range u.Edges.Roles {
		roles = append(roles, r.Code)
	}
	return service.UserRow{
		ID:           uint32(u.ID),
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		Active:       u.Active,
		Roles:        roles,
		Email:        u.Email,
		DisplayName:  u.DisplayName,
		LastLoginAt:  u.LastLoginAt,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}
