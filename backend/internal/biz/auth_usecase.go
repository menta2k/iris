package biz

import (
	"context"
	"sort"
	"strings"
)

// Login status values returned by AuthUsecase.Login / VerifyMFA.
const (
	// LoginAuthenticated means the token is fully usable.
	LoginAuthenticated = "authenticated"
	// LoginMFARequired means credentials were valid but a TOTP code is needed.
	LoginMFARequired = "mfa_required"
	// LoginMFAEnrollmentRequired means the user must enroll MFA before access.
	LoginMFAEnrollmentRequired = "mfa_enrollment_required"
)

// LoginResult is the outcome of a login or MFA-verification step.
type LoginResult struct {
	Token       string
	Status      string
	User        *IrisUser
	Permissions []string
}

// AuthUsecase implements password login, MFA-gated sessions, and session-token
// resolution for the auth middleware. It is the SessionResolver the middleware
// depends on.
type AuthUsecase struct {
	repo            IdentityRepo
	mfa             MFAProvider
	sessions        *SessionManager
	auditor         *Auditor
	mfaGlobalForced bool
}

// NewAuthUsecase constructs the use case. mfaGlobalForced mirrors the
// deployment-wide auth.mfa_required setting so login can drive enrollment.
func NewAuthUsecase(repo IdentityRepo, mfa MFAProvider, sessions *SessionManager, auditor *Auditor, mfaGlobalForced bool) *AuthUsecase {
	return &AuthUsecase{repo: repo, mfa: mfa, sessions: sessions, auditor: auditor, mfaGlobalForced: mfaGlobalForced}
}

// Login verifies email + password and issues a session token. When MFA is
// required the token is partially-authenticated (MFAVerified=false) and the
// status tells the client whether to verify an existing enrollment or enroll.
func (uc *AuthUsecase) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	invalid := Unauthorized("INVALID_CREDENTIALS", "invalid email or password")

	user, err := uc.repo.FindUserByEmail(ctx, email)
	if err != nil {
		// Do not leak whether the account exists.
		return nil, invalid
	}
	if !CheckPassword(user.PasswordHash, password) {
		uc.auditAs(ctx, user, "auth.login", AuditFailure, map[string]any{"reason": "bad_password"})
		return nil, invalid
	}
	if !user.CanAuthenticate() {
		uc.auditAs(ctx, user, "auth.login", AuditFailure, map[string]any{"reason": "not_active", "status": user.Status})
		return nil, Forbidden("USER_NOT_ACTIVE", "user is not permitted to authenticate")
	}

	enrolled, err := uc.mfa.Enrolled(ctx, user.ID)
	if err != nil {
		return nil, Internal(err, "check mfa enrollment")
	}
	needMFA := uc.mfaGlobalForced || user.MFARequired

	status := LoginAuthenticated
	mfaVerified := true
	switch {
	case needMFA && !enrolled:
		status, mfaVerified = LoginMFAEnrollmentRequired, false
	case needMFA && enrolled:
		status, mfaVerified = LoginMFARequired, false
	}

	token, err := uc.sessions.Issue(user.ID, user.Email, mfaVerified)
	if err != nil {
		return nil, err
	}
	uc.auditAs(ctx, user, "auth.login", AuditSuccess, map[string]any{"status": status})
	return uc.result(token, status, user), nil
}

// VerifyMFA validates a TOTP code for the partially-authenticated caller and
// upgrades the session to fully-authenticated.
func (uc *AuthUsecase) VerifyMFA(ctx context.Context, code string) (*LoginResult, error) {
	id, err := RequireIdentity(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := uc.mfa.Verify(ctx, id.UserID, code); err != nil {
		uc.audit(ctx, "auth.mfa_verify", AuditFailure, nil)
		return nil, err
	}
	user, err := uc.repo.FindUserByEmail(ctx, id.Email)
	if err != nil {
		return nil, Unauthorized("UNAUTHENTICATED", "invalid session")
	}
	if !user.CanAuthenticate() {
		return nil, Forbidden("USER_NOT_ACTIVE", "user is not permitted to authenticate")
	}
	token, err := uc.sessions.Issue(user.ID, user.Email, true)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, "auth.mfa_verify", AuditSuccess, nil)
	return uc.result(token, LoginAuthenticated, user), nil
}

// IssueVerifiedToken mints a fully-authenticated token for the calling
// identity. Used after a first-login MFA enrollment is confirmed.
func (uc *AuthUsecase) IssueVerifiedToken(ctx context.Context) (string, error) {
	id, err := RequireIdentity(ctx)
	if err != nil {
		return "", err
	}
	return uc.sessions.Issue(id.UserID, id.Email, true)
}

// CurrentUser returns the calling user's fresh profile and effective
// permissions, reloaded from storage.
func (uc *AuthUsecase) CurrentUser(ctx context.Context) (*IrisUser, []string, error) {
	id, err := RequireIdentity(ctx)
	if err != nil {
		return nil, nil, err
	}
	user, err := uc.repo.FindUserByEmail(ctx, id.Email)
	if err != nil {
		return nil, nil, Unauthorized("UNAUTHENTICATED", "invalid session")
	}
	return user, permissionList(user.Roles), nil
}

// ChangePassword updates the calling user's own password after verifying the
// current one.
func (uc *AuthUsecase) ChangePassword(ctx context.Context, current, next string) error {
	id, err := RequireIdentity(ctx)
	if err != nil {
		return err
	}
	user, err := uc.repo.FindUserByEmail(ctx, id.Email)
	if err != nil {
		return Unauthorized("UNAUTHENTICATED", "invalid session")
	}
	if !CheckPassword(user.PasswordHash, current) {
		uc.audit(ctx, "auth.change_password", AuditFailure, map[string]any{"reason": "bad_current"})
		return Unauthorized("INVALID_CREDENTIALS", "current password is incorrect")
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	if err := uc.repo.SetPassword(ctx, user.ID, hash); err != nil {
		uc.audit(ctx, "auth.change_password", AuditFailure, nil)
		return err
	}
	uc.audit(ctx, "auth.change_password", AuditSuccess, nil)
	return nil
}

// Resolve validates a session token and returns the identity for the auth
// middleware. The MFAVerified flag comes from the token claim; per-user MFA
// requirement is surfaced so the middleware can gate combined with the
// deployment-wide setting.
func (uc *AuthUsecase) Resolve(ctx context.Context, token string) (*Identity, error) {
	claims, err := uc.sessions.Parse(token)
	if err != nil {
		return nil, err
	}
	user, err := uc.repo.FindUserByEmail(ctx, claims.Email)
	if err != nil {
		return nil, Unauthorized("UNAUTHENTICATED", "invalid session")
	}
	if !user.CanAuthenticate() {
		return nil, Unauthorized("USER_NOT_ACTIVE", "user is not permitted to authenticate")
	}
	return &Identity{
		UserID:      user.ID,
		Email:       user.Email,
		Roles:       user.Roles,
		Permissions: ResolvePermissions(user.Roles, nil),
		MFARequired: user.MFARequired,
		MFAVerified: claims.MFAVerified,
	}, nil
}

func (uc *AuthUsecase) result(token, status string, user *IrisUser) *LoginResult {
	return &LoginResult{Token: token, Status: status, User: user, Permissions: permissionList(user.Roles)}
}

// permissionList returns the user's effective permissions as a sorted slice.
func permissionList(roles []string) []string {
	set := ResolvePermissions(roles, nil)
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, string(p))
	}
	sort.Strings(out)
	return out
}

func (uc *AuthUsecase) audit(ctx context.Context, op string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, "auth", "", outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}

// auditAs records an auth event attributed to the given user even when the
// request context carries no identity yet (e.g. during login).
func (uc *AuthUsecase) auditAs(ctx context.Context, user *IrisUser, op string, outcome AuditOutcome, summary map[string]any) {
	if user != nil && IdentityFrom(ctx) == nil {
		ctx = WithIdentity(ctx, &Identity{UserID: user.ID, Email: user.Email})
	}
	uc.audit(ctx, op, outcome, summary)
}
