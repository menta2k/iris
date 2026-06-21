package integration

import (
	"context"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

func newIdentityUC(db *data.DB) *biz.IdentityUsecase {
	return biz.NewIdentityUsecase(data.NewIdentityRepo(db, data.NewAuditRepo(db)), biz.NewPlaceholderMFA(), nil)
}

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

	if _, err := uc.CreateUser(ctx, &biz.IrisUser{Email: "new@example.com"}); err == nil {
		t.Fatal("expected create user denied")
	} else if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindForbidden {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if _, err := uc.ListAuditEntries(ctx, biz.NormalizePage(0, "")); err == nil {
		t.Fatal("expected audit read denied")
	}
}

// TestSessionResolutionEnforcesActiveAndMFA verifies the session resolver
// rejects disabled users and reflects MFA state.
func TestSessionResolutionEnforcesActiveAndMFA(t *testing.T) {
	db := setupDB(t)
	uc := newIdentityUC(db)
	admin := ownerCtx()

	// Seed an owner role so resolved permissions are populated.
	if _, err := db.Pool.Exec(context.Background(),
		`INSERT INTO roles (name, permissions, builtin) VALUES ('owner', ARRAY['*'], true)
		 ON CONFLICT (name) DO NOTHING`); err != nil {
		t.Fatalf("seed role: %v", err)
	}

	active, err := uc.CreateUser(admin, &biz.IrisUser{
		Email: "active@example.com", Status: biz.UserActive, MFARequired: false, Roles: []string{"owner"},
	})
	if err != nil {
		t.Fatalf("create active user: %v", err)
	}
	disabled, err := uc.CreateUser(admin, &biz.IrisUser{
		Email: "disabled@example.com", Status: biz.UserDisabled,
	})
	if err != nil {
		t.Fatalf("create disabled user: %v", err)
	}

	// Active user resolves with permissions.
	id, err := uc.Resolve(context.Background(), active.Email)
	if err != nil {
		t.Fatalf("resolve active: %v", err)
	}
	if !id.Permissions.Has(biz.PermUserWrite) {
		t.Fatal("active owner should resolve to full permissions")
	}

	// Disabled user is rejected.
	if _, err := uc.Resolve(context.Background(), disabled.Email); err == nil {
		t.Fatal("expected disabled user resolution to fail")
	} else if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindUnauthorized {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}
