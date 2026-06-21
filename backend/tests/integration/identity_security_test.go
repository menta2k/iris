package integration

import (
	"context"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

func newIdentityUC(db *data.DB) *biz.IdentityUsecase {
	return biz.NewIdentityUsecase(data.NewIdentityRepo(db, data.NewAuditRepo(db)), biz.NewPlaceholderMFA(), nil)
}

// newAuthUC builds an AuthUsecase backed by the real TOTP provider and a
// deterministic session secret. mfaForced mirrors auth.mfa_required.
func newAuthUC(db *data.DB, mfaForced bool) *biz.AuthUsecase {
	repo := data.NewIdentityRepo(db, data.NewAuditRepo(db))
	return biz.NewAuthUsecase(repo, biz.NewTOTPMFA(repo, "Iris"),
		biz.NewSessionManager("test-session-secret-0123456789", time.Hour), nil, mfaForced)
}

// seedOwnerRole inserts the built-in owner role (setupDB truncates roles).
func seedOwnerRole(t *testing.T, db *data.DB) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(),
		`INSERT INTO roles (name, permissions, builtin) VALUES ('owner', ARRAY['*'], true)
		 ON CONFLICT (name) DO NOTHING`); err != nil {
		t.Fatalf("seed role: %v", err)
	}
}

const testPassword = "correct-horse-battery-staple"

// TestUnauthorizedUserManagementDenied verifies a non-security caller cannot
// create users or read the audit log.
func TestUnauthorizedUserManagementDenied(t *testing.T) {
	db := setupDB(t)
	uc := newIdentityUC(db)

	// Operator-style identity without user:write or audit:read.
	ctx := biz.WithIdentity(context.Background(), &biz.Identity{
		UserID:      "00000000-0000-0000-0000-000000000002",
		Permissions: biz.NewPermissionSet([]string{string(biz.PermMailRead)}),
		MFAVerified: true,
	})

	if _, err := uc.CreateUser(ctx, &biz.IrisUser{Email: "new@example.com"}, ""); err == nil {
		t.Fatal("expected create user denied")
	} else if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindForbidden {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if _, err := uc.ListAuditEntries(ctx, biz.NormalizePage(0, "")); err == nil {
		t.Fatal("expected audit read denied")
	}
}

// TestAuthLoginAndSessionResolution verifies password login issues a token that
// resolves to the user's permissions, rejects bad passwords, and refuses
// non-active accounts.
func TestAuthLoginAndSessionResolution(t *testing.T) {
	db := setupDB(t)
	uc := newIdentityUC(db)
	auth := newAuthUC(db, false)
	admin := ownerCtx()
	seedOwnerRole(t, db)
	ctx := context.Background()

	active, err := uc.CreateUser(admin, &biz.IrisUser{
		Email: "active@example.com", Status: biz.UserActive, MFARequired: false, Roles: []string{"owner"},
	}, testPassword)
	if err != nil {
		t.Fatalf("create active user: %v", err)
	}

	// Correct credentials → fully-authenticated token (no MFA required).
	res, err := auth.Login(ctx, active.Email, testPassword)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.Status != biz.LoginAuthenticated || res.Token == "" {
		t.Fatalf("expected authenticated login, got %+v", res)
	}

	// Wrong password is rejected without leaking account existence.
	if _, err := auth.Login(ctx, active.Email, "nope"); err == nil {
		t.Fatal("expected wrong password rejected")
	} else if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindUnauthorized {
		t.Fatalf("expected unauthorized, got %v", err)
	}

	// The token resolves to a full-permission identity.
	id, err := auth.Resolve(ctx, res.Token)
	if err != nil {
		t.Fatalf("resolve active token: %v", err)
	}
	if !id.Permissions.Has(biz.PermUserWrite) {
		t.Fatal("active owner should resolve to full permissions")
	}
	if !id.MFAVerified {
		t.Fatal("token issued without MFA requirement should be verified")
	}

	// A tampered token is rejected.
	if _, err := auth.Resolve(ctx, res.Token+"x"); err == nil {
		t.Fatal("expected tampered token rejected")
	}

	// Disabled users cannot log in.
	disabled, err := uc.CreateUser(admin, &biz.IrisUser{
		Email: "disabled@example.com", Status: biz.UserDisabled, Roles: []string{"owner"},
	}, testPassword)
	if err != nil {
		t.Fatalf("create disabled user: %v", err)
	}
	if _, err := auth.Login(ctx, disabled.Email, testPassword); err == nil {
		t.Fatal("expected disabled login rejected")
	} else if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindForbidden {
		t.Fatalf("expected forbidden, got %v", err)
	}
}
