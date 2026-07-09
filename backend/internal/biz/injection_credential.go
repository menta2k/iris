package biz

import (
	"context"
	"strings"
	"time"
)

// InjectionCredential is one API login for the GreenArrow-compatible injection
// listener. Multiple credentials let each sending application authenticate with
// its own username/password; AllowedMailclasses optionally restricts a
// credential to specific mailclasses (empty = any).
type InjectionCredential struct {
	ID       string
	Username string
	// PasswordHash is the bcrypt digest. Internal-only: never mapped to proto or
	// returned to a client.
	PasswordHash string
	Label        string
	Enabled      bool
	// AllowedMailclasses restricts which mailclasses this credential may inject.
	// Empty means no restriction.
	AllowedMailclasses []string
	LastUsedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// AllowsMailclass reports whether this credential may inject the given
// mailclass. An empty AllowedMailclasses list allows any mailclass.
func (c *InjectionCredential) AllowsMailclass(mailclass string) bool {
	if len(c.AllowedMailclasses) == 0 {
		return true
	}
	mailclass = strings.TrimSpace(mailclass)
	for _, m := range c.AllowedMailclasses {
		if strings.EqualFold(m, mailclass) {
			return true
		}
	}
	return false
}

// InjectionCredentialRepo is the persistence boundary for injection API keys.
type InjectionCredentialRepo interface {
	List(ctx context.Context) ([]*InjectionCredential, error)
	Create(ctx context.Context, c *InjectionCredential, passwordHash string) (*InjectionCredential, error)
	// Update edits metadata only (label, enabled, allowed_mailclasses).
	Update(ctx context.Context, c *InjectionCredential) (*InjectionCredential, error)
	SetPassword(ctx context.Context, id, passwordHash string) (*InjectionCredential, error)
	Delete(ctx context.Context, id string) error
	// ByUsername returns the credential (including PasswordHash) for auth, or
	// (nil, nil) when no such username exists.
	ByUsername(ctx context.Context, username string) (*InjectionCredential, error)
	// TouchLastUsed records a successful authentication; best-effort.
	TouchLastUsed(ctx context.Context, id string) error
}

// maxInjectionMailclasses caps the allow-list to keep it sane.
const maxInjectionMailclasses = 64

// normalizeUsername trims and lowercases a credential username so lookups are
// case-insensitive and stable.
func normalizeUsername(u string) string {
	return strings.ToLower(strings.TrimSpace(u))
}

// normalizeMailclasses trims, de-dupes and drops empties from an allow-list,
// preserving order.
func normalizeMailclasses(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, m := range in {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		key := strings.ToLower(m)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, m)
	}
	return out
}

// ValidateForCreate normalizes and checks a new credential (username + list).
func (c *InjectionCredential) ValidateForCreate() error {
	c.Username = normalizeUsername(c.Username)
	if c.Username == "" {
		return Invalid("INJECT_CRED_USERNAME_REQUIRED", "username is required")
	}
	if len(c.Username) > 128 {
		return Invalid("INJECT_CRED_USERNAME_TOO_LONG", "username must be at most 128 characters")
	}
	if len(c.Label) > 200 {
		return Invalid("INJECT_CRED_LABEL_TOO_LONG", "label must be at most 200 characters")
	}
	c.AllowedMailclasses = normalizeMailclasses(c.AllowedMailclasses)
	if len(c.AllowedMailclasses) > maxInjectionMailclasses {
		return Invalid("INJECT_CRED_TOO_MANY_MAILCLASSES", "at most %d mailclasses may be listed", maxInjectionMailclasses)
	}
	return nil
}
