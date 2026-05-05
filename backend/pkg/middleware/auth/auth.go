// Package auth verifies JWT bearer tokens on incoming gRPC/HTTP requests
// and binds the resulting identity to the request context.
//
// White-listing of public endpoints (e.g. Login) is the caller's
// responsibility — wrap this middleware with a selector.
package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"

	appjwt "github.com/menta2k/iris/backend/pkg/jwt"
)

var (
	ErrMissingBearer = errors.New("auth: missing bearer token")
	ErrInvalidToken  = errors.New("auth: invalid token")
	ErrWrongContext  = errors.New("auth: missing transport context")
)

// Verifier is the minimal interface the middleware needs.
type Verifier interface {
	VerifyAccess(token string) (*appjwt.Claims, error)
}

// Identity is the per-request actor information stored in context.
type Identity struct {
	UserID   uint32
	Username string
	Roles    []string
}

type ctxKey int

const identityKey ctxKey = 1

// FromContext returns the identity bound by the middleware, if any.
func FromContext(ctx context.Context) (Identity, bool) {
	v, ok := ctx.Value(identityKey).(Identity)
	return v, ok
}

// WithIdentity returns a new context carrying the identity (used by tests).
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// IdentityFunc is the audit.IdentityFunc-shaped extractor for this package.
func IdentityFunc(ctx context.Context) (uint32, string) {
	if id, ok := FromContext(ctx); ok {
		return id.UserID, id.Username
	}
	return 0, ""
}

// Server returns a Kratos middleware that verifies the access token.
func Server(v Verifier) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return nil, ErrWrongContext
			}
			tok := bearerFromHeader(tr.RequestHeader().Get("authorization"))
			if tok == "" {
				return nil, ErrMissingBearer
			}
			claims, err := v.VerifyAccess(tok)
			if err != nil {
				return nil, ErrInvalidToken
			}
			ctx = WithIdentity(ctx, Identity{
				UserID:   claims.UserID,
				Username: claims.Username,
				Roles:    claims.Roles,
			})
			return handler(ctx, req)
		}
	}
}

func bearerFromHeader(h string) string {
	const prefix = "Bearer "
	if len(h) < len(prefix) {
		return ""
	}
	if !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
