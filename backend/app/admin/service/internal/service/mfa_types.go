package service

import (
	"context"
	"time"
)

// MFA credential kinds (mirror the ent enum).
const (
	MFAKindTOTP     = "totp"
	MFAKindWebAuthn = "webauthn"
	MFAKindBackup   = "backup_code"
)

// MFACredentialRow is the data-layer view of one enrolled second factor.
// The Secret field's meaning depends on Kind (see the ent schema):
// AES-GCM ciphertext for totp, JSON for webauthn, bcrypt hash for backup_code.
type MFACredentialRow struct {
	ID          uint32
	UserID      uint32
	Kind        string
	Secret      string
	Label       string
	Status      string
	SignCount   uint32
	CreatedAt   time.Time
	ConfirmedAt *time.Time
	UsedAt      *time.Time
}

// MFAStore is the data-layer contract for MFA credentials.
type MFAStore interface {
	// HasActiveMFA reports whether the user has an active totp or webauthn
	// credential (backup codes alone don't gate login).
	HasActiveMFA(ctx context.Context, userID uint32) (bool, error)
	// CountActive counts active credentials of a kind for a user.
	CountActive(ctx context.Context, userID uint32, kind string) (int, error)
	// ListActive returns the active credentials of a kind for a user.
	ListActive(ctx context.Context, userID uint32, kind string) ([]MFACredentialRow, error)
	// GetActiveTOTP returns the user's active TOTP credential, or nil.
	GetActiveTOTP(ctx context.Context, userID uint32) (*MFACredentialRow, error)
	// GetActive returns one active credential owned by the user, or nil.
	GetActive(ctx context.Context, userID, id uint32) (*MFACredentialRow, error)
	Create(ctx context.Context, in MFACredentialRow) (*MFACredentialRow, error)
	// Disable soft-disables one credential by id.
	Disable(ctx context.Context, id uint32) error
	// DisableByKind disables every active credential of a kind for a user
	// (e.g. replacing the backup-code set, or re-enrolling TOTP).
	DisableByKind(ctx context.Context, userID uint32, kind string) error
	// DisableAll disables every active credential for a user (self disable /
	// admin reset).
	DisableAll(ctx context.Context, userID uint32) error
	// MarkUsed records single-use consumption of a backup code (sets used_at
	// + disables it).
	MarkUsed(ctx context.Context, id uint32) error
	// UpdateSecret persists a WebAuthn credential's re-marshalled blob and
	// signature counter after a successful login (clone-detection state).
	UpdateSecret(ctx context.Context, id uint32, secret string, signCount uint32) error
}

// MFASessionStore is a short-lived key/value store for the stateful bits of
// MFA: the pending TOTP-enroll secret and WebAuthn ceremony session data.
// Backed by Redis when configured, else an in-memory TTL map.
type MFASessionStore interface {
	Put(ctx context.Context, key, value string, ttl time.Duration) error
	// Get reads without consuming.
	Get(ctx context.Context, key string) (string, bool, error)
	// GetDel reads and atomically deletes (single-use).
	GetDel(ctx context.Context, key string) (string, bool, error)
}
