package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	appjwt "github.com/menta2k/iris/backend/pkg/jwt"
)

// --- fakes ------------------------------------------------------------------

type memSessions struct{ m map[string]string }

func newMemSessions() *memSessions { return &memSessions{m: map[string]string{}} }

func (s *memSessions) Put(_ context.Context, k, v string, _ time.Duration) error {
	s.m[k] = v
	return nil
}
func (s *memSessions) Get(_ context.Context, k string) (string, bool, error) {
	v, ok := s.m[k]
	return v, ok, nil
}
func (s *memSessions) GetDel(_ context.Context, k string) (string, bool, error) {
	v, ok := s.m[k]
	delete(s.m, k)
	return v, ok, nil
}

type memMFAStore struct {
	rows []MFACredentialRow
	next uint32
}

func (m *memMFAStore) active(userID uint32, kind string) []MFACredentialRow {
	var out []MFACredentialRow
	for _, r := range m.rows {
		if r.UserID == userID && r.Status == "active" && (kind == "" || r.Kind == kind) {
			out = append(out, r)
		}
	}
	return out
}
func (m *memMFAStore) HasActiveMFA(_ context.Context, userID uint32) (bool, error) {
	return len(m.active(userID, MFAKindTOTP))+len(m.active(userID, MFAKindWebAuthn)) > 0, nil
}
func (m *memMFAStore) CountActive(_ context.Context, userID uint32, kind string) (int, error) {
	return len(m.active(userID, kind)), nil
}
func (m *memMFAStore) ListActive(_ context.Context, userID uint32, kind string) ([]MFACredentialRow, error) {
	return m.active(userID, kind), nil
}
func (m *memMFAStore) GetActiveTOTP(_ context.Context, userID uint32) (*MFACredentialRow, error) {
	a := m.active(userID, MFAKindTOTP)
	if len(a) == 0 {
		return nil, nil
	}
	r := a[len(a)-1]
	return &r, nil
}
func (m *memMFAStore) GetActive(_ context.Context, userID, id uint32) (*MFACredentialRow, error) {
	for _, r := range m.rows {
		if r.ID == id && r.UserID == userID && r.Status == "active" {
			rr := r
			return &rr, nil
		}
	}
	return nil, nil
}
func (m *memMFAStore) Create(_ context.Context, in MFACredentialRow) (*MFACredentialRow, error) {
	m.next++
	in.ID = m.next
	in.Status = "active"
	in.CreatedAt = time.Now()
	m.rows = append(m.rows, in)
	return &in, nil
}
func (m *memMFAStore) setStatus(pred func(MFACredentialRow) bool) {
	for i := range m.rows {
		if pred(m.rows[i]) {
			m.rows[i].Status = "disabled"
		}
	}
}
func (m *memMFAStore) Disable(_ context.Context, id uint32) error {
	m.setStatus(func(r MFACredentialRow) bool { return r.ID == id })
	return nil
}
func (m *memMFAStore) DisableByKind(_ context.Context, userID uint32, kind string) error {
	m.setStatus(func(r MFACredentialRow) bool {
		return r.UserID == userID && r.Kind == kind && r.Status == "active"
	})
	return nil
}
func (m *memMFAStore) DisableAll(_ context.Context, userID uint32) error {
	m.setStatus(func(r MFACredentialRow) bool { return r.UserID == userID && r.Status == "active" })
	return nil
}
func (m *memMFAStore) MarkUsed(_ context.Context, id uint32) error {
	m.setStatus(func(r MFACredentialRow) bool { return r.ID == id })
	return nil
}
func (m *memMFAStore) UpdateSecret(_ context.Context, id uint32, secret string, sc uint32) error {
	for i := range m.rows {
		if m.rows[i].ID == id {
			m.rows[i].Secret = secret
			m.rows[i].SignCount = sc
		}
	}
	return nil
}

func newTestMFA(t *testing.T) (*MFAService, *memMFAStore, *appjwt.Issuer) {
	t.Helper()
	iss, err := appjwt.NewIssuer(appjwt.Config{
		AccessSecret:  []byte(strings.Repeat("a", 32)),
		RefreshSecret: []byte(strings.Repeat("b", 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &memMFAStore{}
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	svc := NewMFAService(store, newMemSessions(), iss, nil, key, nil, 12)
	return svc, store, iss
}

// --- tests ------------------------------------------------------------------

func TestTOTPEnrollConfirmAndVerify(t *testing.T) {
	svc, store, iss := newTestMFA(t)
	ctx := context.Background()

	start, err := svc.EnrollTOTPStart(ctx, 1, "alice")
	if err != nil {
		t.Fatalf("enroll start: %v", err)
	}
	code, err := totp.GenerateCode(start.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	codes, err := svc.EnrollTOTPConfirm(ctx, 1, start.OperationID, code)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(codes) != backupCodeCount {
		t.Fatalf("got %d backup codes, want %d", len(codes), backupCodeCount)
	}
	if n, _ := store.CountActive(ctx, 1, MFAKindTOTP); n != 1 {
		t.Fatalf("active totp = %d, want 1", n)
	}

	// Verify a fresh code via the challenge flow.
	challenge, _, _ := iss.IssueMFAChallenge(1, "alice", []string{"admin"}, time.Now())
	freshCode, _ := totp.GenerateCode(start.Secret, time.Now())
	resp, err := svc.VerifyChallenge(ctx, challenge, freshCode, "", "1.2.3.4")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("expected tokens after successful MFA verify")
	}

	// Wrong code rejected.
	challenge2, _, _ := iss.IssueMFAChallenge(1, "alice", nil, time.Now())
	if _, err := svc.VerifyChallenge(ctx, challenge2, "000000", "", "1.2.3.4"); err != ErrMFAInvalidCode {
		t.Fatalf("wrong code: expected ErrMFAInvalidCode, got %v", err)
	}
}

func TestBackupCodeSingleUse(t *testing.T) {
	svc, _, iss := newTestMFA(t)
	ctx := context.Background()

	start, _ := svc.EnrollTOTPStart(ctx, 1, "alice")
	code, _ := totp.GenerateCode(start.Secret, time.Now())
	backups, err := svc.EnrollTOTPConfirm(ctx, 1, start.OperationID, code)
	if err != nil {
		t.Fatal(err)
	}
	one := backups[0]

	ch1, _, _ := iss.IssueMFAChallenge(1, "alice", nil, time.Now())
	if _, err := svc.VerifyChallenge(ctx, ch1, "", one, "1.2.3.4"); err != nil {
		t.Fatalf("first backup use should succeed: %v", err)
	}
	ch2, _, _ := iss.IssueMFAChallenge(1, "alice", nil, time.Now())
	if _, err := svc.VerifyChallenge(ctx, ch2, "", one, "1.2.3.4"); err != ErrMFAInvalidCode {
		t.Fatalf("reused backup code should fail, got %v", err)
	}
}

func TestEnrollTOTPRequiresKey(t *testing.T) {
	iss, _ := appjwt.NewIssuer(appjwt.Config{
		AccessSecret:  []byte(strings.Repeat("a", 32)),
		RefreshSecret: []byte(strings.Repeat("b", 32)),
	})
	svc := NewMFAService(&memMFAStore{}, newMemSessions(), iss, nil, nil, nil, 4)
	if _, err := svc.EnrollTOTPStart(context.Background(), 1, "alice"); err != ErrMFANotConfigured {
		t.Fatalf("expected ErrMFANotConfigured, got %v", err)
	}
}

func TestVerifyChallengeRejectsBadToken(t *testing.T) {
	svc, _, _ := newTestMFA(t)
	if _, err := svc.VerifyChallenge(context.Background(), "garbage", "123456", "", ""); err != ErrMFAChallengeInvalid {
		t.Fatalf("expected ErrMFAChallengeInvalid, got %v", err)
	}
}
