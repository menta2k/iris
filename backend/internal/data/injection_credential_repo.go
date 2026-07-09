package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/menta2k/iris/backend/internal/biz"
)

// InjectionCredentialRepo persists GreenArrow-compatible injection API keys.
type InjectionCredentialRepo struct {
	db *DB
}

// NewInjectionCredentialRepo constructs the repository.
func NewInjectionCredentialRepo(db *DB) *InjectionCredentialRepo {
	return &InjectionCredentialRepo{db: db}
}

var _ biz.InjectionCredentialRepo = (*InjectionCredentialRepo)(nil)

const injectionCredentialCols = `id, username, password_hash, label, enabled, allowed_mailclasses, last_used_at, created_at, updated_at`

func scanInjectionCredential(row interface{ Scan(...any) error }) (*biz.InjectionCredential, error) {
	c := &biz.InjectionCredential{}
	if err := row.Scan(&c.ID, &c.Username, &c.PasswordHash, &c.Label, &c.Enabled,
		&c.AllowedMailclasses, &c.LastUsedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return c, nil
}

// List returns all credentials, newest first.
func (r *InjectionCredentialRepo) List(ctx context.Context) ([]*biz.InjectionCredential, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+injectionCredentialCols+` FROM injection_credentials ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list injection_credentials: %w", err)
	}
	defer rows.Close()
	var out []*biz.InjectionCredential
	for rows.Next() {
		c, err := scanInjectionCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("scan injection_credential: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Create inserts a credential with the given bcrypt hash.
func (r *InjectionCredentialRepo) Create(ctx context.Context, c *biz.InjectionCredential, passwordHash string) (*biz.InjectionCredential, error) {
	mailclasses := c.AllowedMailclasses
	if mailclasses == nil {
		mailclasses = []string{}
	}
	out, err := scanInjectionCredential(r.db.Pool.QueryRow(ctx, `
		INSERT INTO injection_credentials (username, password_hash, label, enabled, allowed_mailclasses)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+injectionCredentialCols,
		c.Username, passwordHash, c.Label, c.Enabled, mailclasses))
	if err != nil {
		return nil, mapConstraint(err, "injection_credential")
	}
	return out, nil
}

// Update edits metadata (label, enabled, allowed_mailclasses) by id.
func (r *InjectionCredentialRepo) Update(ctx context.Context, c *biz.InjectionCredential) (*biz.InjectionCredential, error) {
	mailclasses := c.AllowedMailclasses
	if mailclasses == nil {
		mailclasses = []string{}
	}
	out, err := scanInjectionCredential(r.db.Pool.QueryRow(ctx, `
		UPDATE injection_credentials
		SET label = $2, enabled = $3, allowed_mailclasses = $4, updated_at = now()
		WHERE id = $1
		RETURNING `+injectionCredentialCols,
		c.ID, c.Label, c.Enabled, mailclasses))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.NotFound("INJECT_CRED_NOT_FOUND", "injection credential %s not found", c.ID)
	}
	if err != nil {
		return nil, mapConstraint(err, "injection_credential")
	}
	return out, nil
}

// SetPassword rotates the bcrypt hash for a credential.
func (r *InjectionCredentialRepo) SetPassword(ctx context.Context, id, passwordHash string) (*biz.InjectionCredential, error) {
	out, err := scanInjectionCredential(r.db.Pool.QueryRow(ctx, `
		UPDATE injection_credentials
		SET password_hash = $2, updated_at = now()
		WHERE id = $1
		RETURNING `+injectionCredentialCols,
		id, passwordHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.NotFound("INJECT_CRED_NOT_FOUND", "injection credential %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("set injection_credential password: %w", err)
	}
	return out, nil
}

// Delete removes a credential by id.
func (r *InjectionCredentialRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM injection_credentials WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete injection_credential: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("INJECT_CRED_NOT_FOUND", "injection credential %s not found", id)
	}
	return nil
}

// ByUsername returns the credential for authentication, or (nil, nil) when the
// username does not exist.
func (r *InjectionCredentialRepo) ByUsername(ctx context.Context, username string) (*biz.InjectionCredential, error) {
	out, err := scanInjectionCredential(r.db.Pool.QueryRow(ctx,
		`SELECT `+injectionCredentialCols+` FROM injection_credentials WHERE username = $1`, username))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("injection_credential by username: %w", err)
	}
	return out, nil
}

// TouchLastUsed records a successful authentication timestamp.
func (r *InjectionCredentialRepo) TouchLastUsed(ctx context.Context, id string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE injection_credentials SET last_used_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("touch injection_credential last_used: %w", err)
	}
	return nil
}
