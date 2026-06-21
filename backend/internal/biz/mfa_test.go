package biz

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTOTPGenerateAndVerify(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	now := time.Now()
	code := hotp(secret, uint64(now.Unix())/totpPeriod)
	if len(code) != totpDigits {
		t.Fatalf("code length = %d, want %d", len(code), totpDigits)
	}
	if !VerifyTOTP(secret, code, now) {
		t.Fatalf("valid code %q did not verify", code)
	}
	// A code from a far-away time window must not verify.
	stale := hotp(secret, uint64(now.Add(-10*time.Minute).Unix())/totpPeriod)
	if VerifyTOTP(secret, stale, now) {
		t.Fatalf("stale code %q should not verify", stale)
	}
	if VerifyTOTP(secret, "000000", now) && code != "000000" {
		t.Fatalf("arbitrary code should not verify")
	}
}

func TestTOTPProvisioningURI(t *testing.T) {
	uri := TOTPProvisioningURI("ABC234", "user@example.com", "Iris")
	for _, want := range []string{"otpauth://totp/", "secret=ABC234", "issuer=Iris", "period=30", "digits=6"} {
		if !strings.Contains(uri, want) {
			t.Fatalf("provisioning uri %q missing %q", uri, want)
		}
	}
}

// memMFAStore is an in-memory MFASecretStore for the provider flow test.
type memMFAStore struct {
	secret   string
	enrolled bool
}

func (m *memMFAStore) GetMFA(context.Context, string) (string, bool, error) {
	return m.secret, m.enrolled, nil
}
func (m *memMFAStore) SetMFASecret(_ context.Context, _, s string) error {
	m.secret, m.enrolled = s, false
	return nil
}
func (m *memMFAStore) MarkMFAEnrolled(context.Context, string) error { m.enrolled = true; return nil }
func (m *memMFAStore) ClearMFA(context.Context, string) error {
	m.secret, m.enrolled = "", false
	return nil
}

func TestTOTPEnrollmentFlow(t *testing.T) {
	store := &memMFAStore{}
	mgr := NewTOTPMFA(store, "Iris")
	ctx := context.Background()

	// Not enrolled initially; Verify fails before enrollment.
	if ok, _ := mgr.Enrolled(ctx, "u1"); ok {
		t.Fatal("should not be enrolled before begin")
	}

	enr, err := mgr.BeginEnrollment(ctx, "u1", "u1@example.com")
	if err != nil {
		t.Fatalf("begin enrollment: %v", err)
	}
	if enr.Secret == "" || !strings.Contains(enr.URI, "otpauth://") {
		t.Fatalf("unexpected enrollment material: %+v", enr)
	}
	// Still not enrolled until confirmed.
	if ok, _ := mgr.Enrolled(ctx, "u1"); ok {
		t.Fatal("should not be enrolled before confirm")
	}

	// Wrong code is rejected.
	if err := mgr.ConfirmEnrollment(ctx, "u1", "000001"); err == nil {
		t.Fatal("expected wrong code to be rejected")
	}

	// Correct current code activates enrollment.
	code := hotp(enr.Secret, uint64(time.Now().Unix())/totpPeriod)
	if err := mgr.ConfirmEnrollment(ctx, "u1", code); err != nil {
		t.Fatalf("confirm enrollment: %v", err)
	}
	if ok, _ := mgr.Enrolled(ctx, "u1"); !ok {
		t.Fatal("should be enrolled after confirm")
	}
	// Verify (step-up) now succeeds with a current code.
	if ok, err := mgr.Verify(ctx, "u1", code); err != nil || !ok {
		t.Fatalf("verify after enrollment: ok=%v err=%v", ok, err)
	}

	// Disable clears enrollment.
	if err := mgr.DisableMFA(ctx, "u1"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if ok, _ := mgr.Enrolled(ctx, "u1"); ok {
		t.Fatal("should not be enrolled after disable")
	}
}
