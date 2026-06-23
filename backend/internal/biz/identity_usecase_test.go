package biz

import (
	"context"
	"strings"
	"testing"
)

// fakeIdentityRepo is a minimal IdentityRepo for usecase tests. Only the
// methods under test do real work; the rest return zero values.
type fakeIdentityRepo struct {
	setPasswordID   string
	setPasswordHash string
	setPasswordErr  error
}

func (f *fakeIdentityRepo) CreateUser(context.Context, *IrisUser) (*IrisUser, error) {
	return nil, nil
}
func (f *fakeIdentityRepo) UpdateUser(context.Context, string, *IrisUser) (*IrisUser, error) {
	return nil, nil
}
func (f *fakeIdentityRepo) ListUsers(context.Context, Page) ([]*IrisUser, error) {
	return nil, nil
}
func (f *fakeIdentityRepo) FindUserByEmail(context.Context, string) (*IrisUser, error) {
	return nil, nil
}
func (f *fakeIdentityRepo) SetUserStatus(context.Context, string, string) error { return nil }
func (f *fakeIdentityRepo) SetPassword(_ context.Context, id, hash string) error {
	f.setPasswordID = id
	f.setPasswordHash = hash
	return f.setPasswordErr
}
func (f *fakeIdentityRepo) CountUsers(context.Context) (int, error) { return 0, nil }
func (f *fakeIdentityRepo) ListAuditEntries(context.Context, Page) ([]*AuditEntry, error) {
	return nil, nil
}

func TestResetUserPasswordRequiresPermission(t *testing.T) {
	repo := &fakeIdentityRepo{}
	uc := NewIdentityUsecase(repo, nil, nil)
	ctx := WithIdentity(context.Background(), &Identity{
		Permissions: NewPermissionSet(nil), MFAVerified: true,
	})
	if err := uc.ResetUserPassword(ctx, "u1", "a-very-strong-password"); err == nil {
		t.Fatal("expected permission denied without user:write")
	}
	if repo.setPasswordHash != "" {
		t.Fatal("password must not be set when permission is denied")
	}
}

func TestResetUserPasswordRejectsWeakPassword(t *testing.T) {
	repo := &fakeIdentityRepo{}
	uc := NewIdentityUsecase(repo, nil, nil)
	if err := uc.ResetUserPassword(ownerCtx(), "u1", "short"); err == nil {
		t.Fatal("expected weak password to be rejected")
	}
	if repo.setPasswordHash != "" {
		t.Fatal("weak password must not reach the repo")
	}
}

func TestResetUserPasswordSuccess(t *testing.T) {
	repo := &fakeIdentityRepo{}
	w := &captureWriter{}
	uc := NewIdentityUsecase(repo, nil, NewAuditor(w))

	if err := uc.ResetUserPassword(ownerCtx(), "u1", "a-very-strong-password"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.setPasswordID != "u1" || repo.setPasswordHash == "" {
		t.Fatalf("expected SetPassword(u1, <hash>), got id=%q hash=%q",
			repo.setPasswordID, repo.setPasswordHash)
	}
	// The stored value must be a bcrypt digest, never the plaintext.
	if !strings.HasPrefix(repo.setPasswordHash, "$2") {
		t.Fatalf("expected a bcrypt hash, got %q", repo.setPasswordHash)
	}
	if strings.Contains(repo.setPasswordHash, "a-very-strong-password") {
		t.Fatal("plaintext password must not be stored")
	}
	// Distinct audit op, attributed to the caller, no plaintext in the summary.
	if len(w.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(w.events))
	}
	e := w.events[0]
	if e.Operation != "user.password_reset" || e.Outcome != AuditSuccess || e.TargetID != "u1" {
		t.Fatalf("unexpected audit event: %+v", e)
	}
	if e.ActorUserID != "tester" {
		t.Fatalf("audit not attributed to caller: %q", e.ActorUserID)
	}
}
