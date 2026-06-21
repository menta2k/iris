package biz

import (
	"context"
	"strings"
)

// IdentityRepo is the persistence boundary for users, roles, and audit reads.
type IdentityRepo interface {
	CreateUser(ctx context.Context, u *IrisUser) (*IrisUser, error)
	UpdateUser(ctx context.Context, id string, u *IrisUser) (*IrisUser, error)
	ListUsers(ctx context.Context, page Page) ([]*IrisUser, error)
	FindUserByEmail(ctx context.Context, email string) (*IrisUser, error)
	SetUserStatus(ctx context.Context, id, status string) error
	ListAuditEntries(ctx context.Context, page Page) ([]*AuditEntry, error)
}

// IdentityUsecase implements security administration (US3): user, role, MFA,
// and audit-log management, plus session resolution for the auth middleware.
type IdentityUsecase struct {
	repo    IdentityRepo
	mfa     MFAProvider
	auditor *Auditor
}

// NewIdentityUsecase constructs the use case.
func NewIdentityUsecase(repo IdentityRepo, mfa MFAProvider, auditor *Auditor) *IdentityUsecase {
	return &IdentityUsecase{repo: repo, mfa: mfa, auditor: auditor}
}

// ListUsers returns Iris users after an authorization check.
func (uc *IdentityUsecase) ListUsers(ctx context.Context, page Page) ([]*IrisUser, error) {
	if _, err := RequirePermission(ctx, PermUserRead); err != nil {
		return nil, err
	}
	return uc.repo.ListUsers(ctx, page)
}

// CreateUser validates and persists a new user, auditing the change.
func (uc *IdentityUsecase) CreateUser(ctx context.Context, u *IrisUser) (*IrisUser, error) {
	if _, err := RequirePermission(ctx, PermUserWrite); err != nil {
		return nil, err
	}
	if err := u.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateUser(ctx, u)
	if err != nil {
		uc.audit(ctx, "user.create", "user", u.Email, AuditFailure, map[string]any{"email": u.Email})
		return nil, err
	}
	uc.audit(ctx, "user.create", "user", out.ID, AuditSuccess, map[string]any{
		"email": out.Email, "roles": out.Roles, "mfa_required": out.MFARequired,
	})
	return out, nil
}

// UpdateUser updates a user's profile, status, MFA requirement, and roles. The
// email is immutable (it identifies the account).
func (uc *IdentityUsecase) UpdateUser(ctx context.Context, id string, u *IrisUser) (*IrisUser, error) {
	if _, err := RequirePermission(ctx, PermUserWrite); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("USER_ID_REQUIRED", "user id is required")
	}
	if u.Status == "" {
		u.Status = UserActive
	}
	if !validUserStatus(u.Status) {
		return nil, Invalid("USER_STATUS_INVALID", "status %q is not valid", u.Status)
	}
	out, err := uc.repo.UpdateUser(ctx, id, u)
	if err != nil {
		uc.audit(ctx, "user.update", "user", id, AuditFailure, map[string]any{"status": u.Status})
		return nil, err
	}
	uc.audit(ctx, "user.update", "user", out.ID, AuditSuccess, map[string]any{
		"status": out.Status, "roles": out.Roles, "mfa_required": out.MFARequired,
	})
	return out, nil
}

// SetUserStatus enables, disables, or locks a user.
func (uc *IdentityUsecase) SetUserStatus(ctx context.Context, id, status string) error {
	if _, err := RequirePermission(ctx, PermUserWrite); err != nil {
		return err
	}
	if !validUserStatus(status) {
		return Invalid("USER_STATUS_INVALID", "status %q is not valid", status)
	}
	if err := uc.repo.SetUserStatus(ctx, id, status); err != nil {
		uc.audit(ctx, "user.set_status", "user", id, AuditFailure, map[string]any{"status": status})
		return err
	}
	uc.audit(ctx, "user.set_status", "user", id, AuditSuccess, map[string]any{"status": status})
	return nil
}

// ListAuditEntries returns audit-log entries after an authorization check.
func (uc *IdentityUsecase) ListAuditEntries(ctx context.Context, page Page) ([]*AuditEntry, error) {
	if _, err := RequirePermission(ctx, PermAuditRead); err != nil {
		return nil, err
	}
	return uc.repo.ListAuditEntries(ctx, page)
}

// Resolve implements a simple session-token scheme for the auth middleware: the
// token is the user's email. The user must be active; permissions are derived
// from the user's roles. MFA enrollment is consulted to set MFAVerified. This
// is intentionally pluggable so a real token/session store can replace it
// without changing the middleware.
func (uc *IdentityUsecase) Resolve(ctx context.Context, token string) (*Identity, error) {
	email := strings.ToLower(strings.TrimSpace(token))
	if email == "" {
		return nil, Unauthorized("UNAUTHENTICATED", "empty session token")
	}
	user, err := uc.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, Unauthorized("UNAUTHENTICATED", "invalid session")
	}
	if !user.CanAuthenticate() {
		return nil, Unauthorized("USER_NOT_ACTIVE", "user is not permitted to authenticate")
	}
	enrolled, err := uc.mfa.Enrolled(ctx, user.ID)
	if err != nil {
		return nil, Internal(err, "check mfa enrollment")
	}
	return &Identity{
		UserID:      user.ID,
		Email:       user.Email,
		Roles:       user.Roles,
		Permissions: ResolvePermissions(user.Roles, nil),
		// MFA is considered verified for this session if the user has enrolled
		// or does not require MFA. Real challenge/response is handled by the
		// MFA provider during login.
		MFAVerified: enrolled || !user.MFARequired,
	}, nil
}

// EnrollMFA begins TOTP enrollment for the calling user, returning the secret
// and otpauth provisioning URI to display once. The user must confirm a code
// (ConfirmMFA) to activate it.
func (uc *IdentityUsecase) EnrollMFA(ctx context.Context) (*MFAEnrollment, error) {
	id, err := RequireIdentity(ctx)
	if err != nil {
		return nil, err
	}
	mgr, ok := uc.mfa.(MFAManager)
	if !ok {
		return nil, FailedPrecondition("MFA_UNSUPPORTED", "mfa enrollment is not supported by this deployment")
	}
	out, err := mgr.BeginEnrollment(ctx, id.UserID, id.Email)
	if err != nil {
		uc.audit(ctx, "mfa.enroll_begin", "user", id.UserID, AuditFailure, nil)
		return nil, err
	}
	uc.audit(ctx, "mfa.enroll_begin", "user", id.UserID, AuditSuccess, map[string]any{"method": string(MFATOTP)})
	return out, nil
}

// ConfirmMFA verifies a TOTP code against the pending secret and activates MFA
// for the calling user.
func (uc *IdentityUsecase) ConfirmMFA(ctx context.Context, code string) error {
	id, err := RequireIdentity(ctx)
	if err != nil {
		return err
	}
	mgr, ok := uc.mfa.(MFAManager)
	if !ok {
		return FailedPrecondition("MFA_UNSUPPORTED", "mfa enrollment is not supported by this deployment")
	}
	if err := mgr.ConfirmEnrollment(ctx, id.UserID, code); err != nil {
		uc.audit(ctx, "mfa.enroll_confirm", "user", id.UserID, AuditFailure, nil)
		return err
	}
	uc.audit(ctx, "mfa.enroll_confirm", "user", id.UserID, AuditSuccess, nil)
	return nil
}

// DisableMFA clears the calling user's MFA enrollment.
func (uc *IdentityUsecase) DisableMFA(ctx context.Context) error {
	id, err := RequireIdentity(ctx)
	if err != nil {
		return err
	}
	mgr, ok := uc.mfa.(MFAManager)
	if !ok {
		return FailedPrecondition("MFA_UNSUPPORTED", "mfa enrollment is not supported by this deployment")
	}
	if err := mgr.DisableMFA(ctx, id.UserID); err != nil {
		return err
	}
	uc.audit(ctx, "mfa.disable", "user", id.UserID, AuditSuccess, nil)
	return nil
}

func (uc *IdentityUsecase) audit(ctx context.Context, op, targetType, targetID string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, targetType, targetID, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}
