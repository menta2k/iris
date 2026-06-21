package biz

import "context"

// Identity is the authenticated caller attached to a request context after the
// auth middleware validates the session.
type Identity struct {
	UserID      string
	Email       string
	Roles       []string
	Permissions PermissionSet
	// MFARequired records whether this user must clear MFA; combined with the
	// deployment-wide requirement by the auth middleware.
	MFARequired bool
	MFAVerified bool
	IPAddress   string
	UserAgent   string
	RequestID   string
}

// Authorize checks that the identity holds the required permission.
func (i *Identity) Authorize(required Permission) error {
	if i == nil {
		return Unauthorized("UNAUTHENTICATED", "no authenticated identity")
	}
	return i.Permissions.Authorize(required)
}

type identityKey struct{}

// WithIdentity attaches an authenticated identity to the context.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityFrom returns the authenticated identity, or nil if unauthenticated.
func IdentityFrom(ctx context.Context) *Identity {
	if id, ok := ctx.Value(identityKey{}).(*Identity); ok {
		return id
	}
	return nil
}

// RequireIdentity returns the identity or an unauthenticated error.
func RequireIdentity(ctx context.Context) (*Identity, error) {
	id := IdentityFrom(ctx)
	if id == nil {
		return nil, Unauthorized("UNAUTHENTICATED", "authentication required")
	}
	return id, nil
}

// RequirePermission returns the identity after checking a permission, or an
// authorization error suitable for returning to the caller.
func RequirePermission(ctx context.Context, required Permission) (*Identity, error) {
	id, err := RequireIdentity(ctx)
	if err != nil {
		return nil, err
	}
	if err := id.Authorize(required); err != nil {
		return nil, err
	}
	return id, nil
}
