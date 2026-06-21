package service

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
)

// SessionResolver validates a bearer/session token and returns the associated
// identity. Implementations are provided by the identity use cases (US3).
type SessionResolver interface {
	Resolve(ctx context.Context, token string) (*biz.Identity, error)
}

// AuthMiddleware authenticates requests and attaches a biz.Identity to the
// context. Health and readiness endpoints are exempt. When dev bypass is
// enabled it injects a full-permission identity so local development does not
// require a configured identity store.
func AuthMiddleware(cfg conf.Auth, resolver SessionResolver) middleware.Middleware {
	owner := &biz.Identity{
		// A valid (nil) UUID so audit/service-control inserts into UUID columns
		// succeed for the synthetic dev-bypass identity.
		UserID:      "00000000-0000-0000-0000-000000000000",
		Email:       "dev@localhost",
		Roles:       []string{biz.RoleOwner},
		Permissions: biz.NewPermissionSet([]string{string(biz.PermAll)}),
		MFAVerified: true,
	}
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			if ok && isPublicOperation(tr.Operation()) {
				return handler(ctx, req)
			}

			var id *biz.Identity
			if cfg.DevBypass {
				id = owner
			} else {
				token := bearerToken(tr, ok)
				if token == "" || resolver == nil {
					return nil, mapError(biz.Unauthorized("UNAUTHENTICATED", "authentication required"))
				}
				resolved, err := resolver.Resolve(ctx, token)
				if err != nil {
					return nil, mapError(err)
				}
				id = resolved
			}

			if ok {
				id = enrichIdentity(id, tr)
			}
			// Gate on MFA unless this is a step that completes MFA (verify or
			// first-login enrollment) or the caller's own profile/logout — those
			// must be reachable with a partially-authenticated token.
			needMFA := cfg.MFARequired || id.MFARequired
			if needMFA && !id.MFAVerified && !(ok && isMFAStepOperation(tr.Operation())) {
				return nil, mapError(biz.Forbidden("MFA_REQUIRED", "multi-factor authentication required"))
			}
			ctx = biz.WithIdentity(ctx, id)
			return handler(ctx, req)
		}
	}
}

// isPublicOperation exempts health/readiness and the login endpoint (which
// exchanges credentials for a token and therefore cannot require one).
func isPublicOperation(op string) bool {
	return strings.Contains(op, "Health") || strings.Contains(op, "Login")
}

// isMFAStepOperation lists operations reachable with a valid but not-yet-MFA-
// verified token: completing the MFA challenge, first-login enrollment, reading
// one's own identity, and logging out.
func isMFAStepOperation(op string) bool {
	switch {
	case strings.HasSuffix(op, "/VerifyMFA"),
		strings.HasSuffix(op, "/EnrollMFA"),
		strings.HasSuffix(op, "/ConfirmMFA"),
		strings.HasSuffix(op, "/CurrentUser"),
		strings.HasSuffix(op, "/Logout"):
		return true
	default:
		return false
	}
}

func bearerToken(tr transport.Transporter, ok bool) string {
	if !ok {
		return ""
	}
	auth := tr.RequestHeader().Get("Authorization")
	if auth == "" {
		auth = tr.RequestHeader().Get("X-Session-Token")
		return auth
	}
	const prefix = "Bearer "
	if strings.HasPrefix(auth, prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}
	return auth
}

// enrichIdentity returns a per-request copy of id annotated with transport
// metadata, leaving any shared base identity untouched.
func enrichIdentity(id *biz.Identity, tr transport.Transporter) *biz.Identity {
	h := tr.RequestHeader()
	clone := *id
	clone.RequestID = h.Get("X-Request-Id")
	clone.UserAgent = h.Get("User-Agent")
	clone.IPAddress = clientIP(h.Get("X-Forwarded-For"))
	return &clone
}

func clientIP(forwarded string) string {
	if forwarded == "" {
		return ""
	}
	if i := strings.IndexByte(forwarded, ','); i >= 0 {
		return strings.TrimSpace(forwarded[:i])
	}
	return strings.TrimSpace(forwarded)
}
