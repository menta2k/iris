package integration

import (
	"context"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestMFALoginFlow exercises the full first-login MFA path against the real
// TOTP provider and repo: a user that requires MFA logs in to a partial token,
// enrolls, then a subsequent login + verify yields a fully-authenticated token.
func TestMFALoginFlow(t *testing.T) {
	db := setupDB(t)
	repo := data.NewIdentityRepo(db, data.NewAuditRepo(db))
	idUC := biz.NewIdentityUsecase(repo, biz.NewTOTPMFA(repo, "Iris"), nil)
	auth := newAuthUC(db, false)
	admin := ownerCtx()
	seedOwnerRole(t, db)
	ctx := context.Background()

	user, err := idUC.CreateUser(admin, &biz.IrisUser{
		Email: "mfa@example.com", Status: biz.UserActive, MFARequired: true, Roles: []string{"owner"},
	}, testPassword)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// First login: MFA required but not enrolled → enrollment-required status
	// and a partial (not MFA-verified) token.
	res, err := auth.Login(ctx, user.Email, testPassword)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.Status != biz.LoginMFAEnrollmentRequired {
		t.Fatalf("expected enrollment required, got %q", res.Status)
	}
	partial, err := auth.Resolve(ctx, res.Token)
	if err != nil {
		t.Fatalf("resolve partial: %v", err)
	}
	if partial.MFAVerified {
		t.Fatal("partial token must not be MFA-verified")
	}

	// Enroll + confirm as the partially-authenticated user.
	self := biz.WithIdentity(ctx, partial)
	enr, err := idUC.EnrollMFA(self)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if err := idUC.ConfirmMFA(self, biz.GenerateTOTP(enr.Secret, time.Now())); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Second login: now enrolled → mfa_required (verify, not enroll).
	res2, err := auth.Login(ctx, user.Email, testPassword)
	if err != nil {
		t.Fatalf("login2: %v", err)
	}
	if res2.Status != biz.LoginMFARequired {
		t.Fatalf("expected mfa_required, got %q", res2.Status)
	}

	// Verify the current code → fully-authenticated token.
	pwLevel, err := auth.Resolve(ctx, res2.Token)
	if err != nil {
		t.Fatalf("resolve pw-level: %v", err)
	}
	vres, err := auth.VerifyMFA(biz.WithIdentity(ctx, pwLevel), biz.GenerateTOTP(enr.Secret, time.Now()))
	if err != nil {
		t.Fatalf("verify mfa: %v", err)
	}
	if vres.Status != biz.LoginAuthenticated {
		t.Fatalf("expected authenticated after verify, got %q", vres.Status)
	}
	full, err := auth.Resolve(ctx, vres.Token)
	if err != nil {
		t.Fatalf("resolve full: %v", err)
	}
	if !full.MFAVerified {
		t.Fatal("verified token must be MFA-verified")
	}

	// A wrong code is rejected.
	if _, err := auth.VerifyMFA(biz.WithIdentity(ctx, pwLevel), "000000"); err == nil {
		t.Fatal("expected wrong mfa code rejected")
	}
}
