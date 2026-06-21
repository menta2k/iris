package integration

import (
	"context"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestMFAEnrollmentReflectedInResolve exercises the real TOTP MFA path through
// the repo: enrolling stores a pending secret, confirming marks the user
// enrolled, and the session resolver then reports MFAVerified.
func TestMFAEnrollmentReflectedInResolve(t *testing.T) {
	db := setupDB(t)
	repo := data.NewIdentityRepo(db, data.NewAuditRepo(db))
	uc := biz.NewIdentityUsecase(repo, biz.NewTOTPMFA(repo, "Iris"), nil)
	admin := ownerCtx()

	if _, err := db.Pool.Exec(context.Background(),
		`INSERT INTO roles (name, permissions, builtin) VALUES ('owner', ARRAY['*'], true)
		 ON CONFLICT (name) DO NOTHING`); err != nil {
		t.Fatalf("seed role: %v", err)
	}
	user, err := uc.CreateUser(admin, &biz.IrisUser{
		Email: "mfa@example.com", Status: biz.UserActive, MFARequired: true, Roles: []string{"owner"},
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ctx := context.Background()

	// Required but not enrolled → not MFA-verified.
	id, err := uc.Resolve(ctx, user.Email)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if id.MFAVerified {
		t.Fatal("should not be MFA-verified before enrollment")
	}

	// The user enrolls themselves (caller identity = the user).
	self := biz.WithIdentity(ctx, &biz.Identity{
		UserID: user.ID, Email: user.Email,
		Permissions: biz.NewPermissionSet([]string{string(biz.PermAll)}), MFAVerified: true,
	})
	enr, err := uc.EnrollMFA(self)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if enr.Secret == "" || enr.URI == "" {
		t.Fatalf("expected enrollment material, got %+v", enr)
	}

	// Pending: secret stored but not yet confirmed.
	secret, enrolled, err := repo.GetMFA(ctx, user.ID)
	if err != nil {
		t.Fatalf("get mfa: %v", err)
	}
	if enrolled || secret != enr.Secret {
		t.Fatalf("expected pending secret %q, got secret=%q enrolled=%v", enr.Secret, secret, enrolled)
	}
	if v, _ := uc.Resolve(ctx, user.Email); v.MFAVerified {
		t.Fatal("pending enrollment must not count as verified")
	}

	// Confirm (mark enrolled) → resolver now reports verified.
	if err := repo.MarkMFAEnrolled(ctx, user.ID); err != nil {
		t.Fatalf("mark enrolled: %v", err)
	}
	if v, _ := uc.Resolve(ctx, user.Email); !v.MFAVerified {
		t.Fatal("should be MFA-verified after enrollment")
	}

	// Disable clears it.
	if err := uc.DisableMFA(self); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if v, _ := uc.Resolve(ctx, user.Email); v.MFAVerified {
		t.Fatal("should not be verified after disable")
	}
}
