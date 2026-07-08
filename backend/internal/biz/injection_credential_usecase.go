package biz

import "context"

// InjectionCredentialUsecase is the operator-facing CRUD for injection API
// credentials (the "Injection API" settings page). Reads require injection:read,
// mutations injection:write. Passwords are bcrypt-hashed and never returned.
type InjectionCredentialUsecase struct {
	repo    InjectionCredentialRepo
	auditor *Auditor
}

// NewInjectionCredentialUsecase constructs the use case.
func NewInjectionCredentialUsecase(repo InjectionCredentialRepo, auditor *Auditor) *InjectionCredentialUsecase {
	return &InjectionCredentialUsecase{repo: repo, auditor: auditor}
}

// List returns all injection credentials (without password material).
func (uc *InjectionCredentialUsecase) List(ctx context.Context) ([]*InjectionCredential, error) {
	if _, err := RequirePermission(ctx, PermInjectionRead); err != nil {
		return nil, err
	}
	items, err := uc.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range items {
		c.PasswordHash = "" // never leak the hash outward
	}
	return items, nil
}

// Create adds a credential. The password is strength-validated and bcrypt-hashed.
func (uc *InjectionCredentialUsecase) Create(ctx context.Context, c *InjectionCredential, password string) (*InjectionCredential, error) {
	if _, err := RequirePermission(ctx, PermInjectionWrite); err != nil {
		return nil, err
	}
	if err := c.ValidateForCreate(); err != nil {
		return nil, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	out, err := uc.repo.Create(ctx, c, hash)
	if err != nil {
		return nil, err
	}
	out.PasswordHash = ""
	uc.audit(ctx, "injection_credential.create", out.ID, AuditSuccess, map[string]any{"username": out.Username})
	return out, nil
}

// Update edits metadata (label, enabled, allowed_mailclasses) by id.
func (uc *InjectionCredentialUsecase) Update(ctx context.Context, c *InjectionCredential) (*InjectionCredential, error) {
	if _, err := RequirePermission(ctx, PermInjectionWrite); err != nil {
		return nil, err
	}
	if c.ID == "" {
		return nil, Invalid("INJECT_CRED_ID_REQUIRED", "id is required")
	}
	if len(c.Label) > 200 {
		return nil, Invalid("INJECT_CRED_LABEL_TOO_LONG", "label must be at most 200 characters")
	}
	c.AllowedMailclasses = normalizeMailclasses(c.AllowedMailclasses)
	if len(c.AllowedMailclasses) > maxInjectionMailclasses {
		return nil, Invalid("INJECT_CRED_TOO_MANY_MAILCLASSES", "at most %d mailclasses may be listed", maxInjectionMailclasses)
	}
	out, err := uc.repo.Update(ctx, c)
	if err != nil {
		return nil, err
	}
	out.PasswordHash = ""
	uc.audit(ctx, "injection_credential.update", out.ID, AuditSuccess, map[string]any{"username": out.Username})
	return out, nil
}

// SetPassword rotates a credential's password.
func (uc *InjectionCredentialUsecase) SetPassword(ctx context.Context, id, password string) (*InjectionCredential, error) {
	if _, err := RequirePermission(ctx, PermInjectionWrite); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("INJECT_CRED_ID_REQUIRED", "id is required")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	out, err := uc.repo.SetPassword(ctx, id, hash)
	if err != nil {
		return nil, err
	}
	out.PasswordHash = ""
	uc.audit(ctx, "injection_credential.set_password", out.ID, AuditSuccess, map[string]any{"username": out.Username})
	return out, nil
}

// Delete removes a credential by id.
func (uc *InjectionCredentialUsecase) Delete(ctx context.Context, id string) error {
	if _, err := RequirePermission(ctx, PermInjectionWrite); err != nil {
		return err
	}
	if err := uc.repo.Delete(ctx, id); err != nil {
		return err
	}
	uc.audit(ctx, "injection_credential.delete", id, AuditSuccess, nil)
	return nil
}

func (uc *InjectionCredentialUsecase) audit(ctx context.Context, op, id string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, "injection_credential", id, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}
