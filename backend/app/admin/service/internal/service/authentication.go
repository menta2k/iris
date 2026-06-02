// Package service holds the gRPC/HTTP service implementations.
//
// AuthenticationService implements login/logout/refresh against a UserStore
// (which abstracts the ent-backed user repository). The service is
// transport-agnostic: the same struct can be exposed over gRPC and HTTP via
// generated kratos service registration.
package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	appcrypto "github.com/menta2k/iris/backend/pkg/crypto"
	appjwt "github.com/menta2k/iris/backend/pkg/jwt"
)

// UserStore is the minimal interface AuthenticationService needs from data.
// It is intentionally small to keep the service decoupled from ent.
type UserStore interface {
	FindByUsername(ctx context.Context, username string) (*UserRow, error)
	RecordLoginSuccess(ctx context.Context, userID uint32, ip string, at time.Time) error
	RecordLoginFailure(ctx context.Context, username string, at time.Time) error
	IsLockedOut(ctx context.Context, userID uint32, at time.Time) (bool, error)
}

// UserRow is the data-layer view of a user, decoupled from ent generated types.
//
// The auth path only reads ID/Username/PasswordHash/Active/Roles; the
// remaining fields are populated by the admin-CRUD repo methods and consumed
// by UserService for the management API.
type UserRow struct {
	ID           uint32
	Username     string
	PasswordHash string
	Active       bool
	Roles        []string
	Email        string
	DisplayName  string
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AuthenticationService implements the auth-related gRPC methods.
type AuthenticationService struct {
	users    UserStore
	jwt      *appjwt.Issuer
	firewall *LoginFirewall // optional; nil disables login-firewall enforcement
	now      func() time.Time
}

// NewAuthenticationService constructs the service. firewall may be nil (no
// login-policy enforcement) — tests and minimal deployments pass nil.
func NewAuthenticationService(users UserStore, issuer *appjwt.Issuer, firewall *LoginFirewall) *AuthenticationService {
	return &AuthenticationService{users: users, jwt: issuer, firewall: firewall, now: time.Now}
}

// LoginRequest is the input to Login.
type LoginRequest struct {
	Username string
	Password string
}

// LoginResponse is the output of a successful Login / RefreshToken.
type LoginResponse struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	UserID       uint32
	Username     string
	Roles        []string
}

// Errors surfaced to the API layer (callers map to gRPC codes).
var (
	ErrInvalidCredentials = errors.New("authentication: invalid credentials")
	ErrAccountInactive    = errors.New("authentication: account inactive")
	ErrAccountLocked      = errors.New("authentication: account locked")
	ErrLoginBlocked       = errors.New("authentication: login blocked by policy")
)

// Login validates a username + password and issues a token pair.
//
// Timing-attack mitigation: the same hash-verify path runs whether or not the
// username exists. We perform a dummy bcrypt compare against a fixed hash
// when the user is missing so the per-request latency is dominated by bcrypt
// either way.
func (s *AuthenticationService) Login(ctx context.Context, req *LoginRequest, clientIP string) (*LoginResponse, error) {
	if req == nil || req.Username == "" || req.Password == "" {
		return nil, ErrInvalidCredentials
	}
	user, err := s.users.FindByUsername(ctx, req.Username)
	if err != nil {
		// Run the dummy compare to keep timing roughly constant.
		_ = appcrypto.VerifyPassword(dummyHash, req.Password)
		_ = s.users.RecordLoginFailure(ctx, req.Username, s.now())
		return nil, ErrInvalidCredentials
	}
	if !user.Active {
		return nil, ErrAccountInactive
	}
	// Login firewall: evaluate global + per-user rules before spending a
	// bcrypt compare or touching the lockout counter, so a blocked source
	// can't probe passwords. A store error fails open (logged) — a DB blip
	// must not lock out every login. The missing-user branch above is left
	// untouched to preserve the timing-attack mitigation (enforcing there
	// would add a query only when the user exists, leaking existence).
	if s.firewall != nil {
		uid := user.ID
		res, ferr := s.firewall.Evaluate(ctx, LoginAttempt{
			Username: req.Username,
			UserID:   &uid,
			IP:       clientIP,
			Now:      s.now(),
		})
		if ferr != nil {
			log.Printf("authentication: login_firewall evaluate error (failing open): %v", ferr)
		} else if !res.Allowed {
			return nil, ErrLoginBlocked
		}
	}
	locked, err := s.users.IsLockedOut(ctx, user.ID, s.now())
	if err != nil {
		return nil, fmt.Errorf("authentication: lockout check: %w", err)
	}
	if locked {
		return nil, ErrAccountLocked
	}
	if err := appcrypto.VerifyPassword(user.PasswordHash, req.Password); err != nil {
		_ = s.users.RecordLoginFailure(ctx, req.Username, s.now())
		return nil, ErrInvalidCredentials
	}

	access, accessExp, err := s.jwt.IssueAccess(user.ID, user.Username, user.Roles, s.now())
	if err != nil {
		return nil, fmt.Errorf("authentication: issue access: %w", err)
	}
	refresh, _, err := s.jwt.IssueRefresh(user.ID, user.Username, s.now())
	if err != nil {
		return nil, fmt.Errorf("authentication: issue refresh: %w", err)
	}
	_ = s.users.RecordLoginSuccess(ctx, user.ID, clientIP, s.now())

	return &LoginResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(time.Until(accessExp).Seconds()),
		UserID:       user.ID,
		Username:     user.Username,
		Roles:        user.Roles,
	}, nil
}

// RefreshToken validates a refresh token and issues a new pair.
func (s *AuthenticationService) RefreshToken(ctx context.Context, refreshToken string) (*LoginResponse, error) {
	claims, err := s.jwt.VerifyRefresh(refreshToken)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	user, err := s.users.FindByUsername(ctx, claims.Username)
	if err != nil || !user.Active {
		return nil, ErrInvalidCredentials
	}
	access, accessExp, err := s.jwt.IssueAccess(user.ID, user.Username, user.Roles, s.now())
	if err != nil {
		return nil, fmt.Errorf("authentication: issue access: %w", err)
	}
	refresh, _, err := s.jwt.IssueRefresh(user.ID, user.Username, s.now())
	if err != nil {
		return nil, fmt.Errorf("authentication: issue refresh: %w", err)
	}
	return &LoginResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(time.Until(accessExp).Seconds()),
		UserID:       user.ID,
		Username:     user.Username,
		Roles:        user.Roles,
	}, nil
}

// dummyHash is a real bcrypt hash of the empty string, used so the
// timing-equalization compare in Login takes a comparable amount of time.
//
// Bcrypt cost matches MinBcryptCost; if the deployment uses a higher cost,
// the dummy compare is slightly faster than a real compare. That's
// acceptable: the goal is to mask "user exists / does not", not to exactly
// equalize against varying configured cost.
var dummyHash = "$2a$12$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
