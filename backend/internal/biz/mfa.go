package biz

import (
	"context"
	"time"
)

// MFAMethod identifies a multi-factor challenge type.
type MFAMethod string

const (
	MFATOTP        MFAMethod = "totp"
	MFAWebAuthn    MFAMethod = "webauthn"
	MFARecoveryKey MFAMethod = "recovery_key"
)

// MFAChallenge represents a pending challenge issued to a user.
type MFAChallenge struct {
	ChallengeID string
	UserID      string
	Method      MFAMethod
}

// MFAProvider issues and verifies multi-factor challenges.
type MFAProvider interface {
	// Enrolled reports whether the user has an active MFA enrollment.
	Enrolled(ctx context.Context, userID string) (bool, error)
	// Challenge issues a new challenge for the user.
	Challenge(ctx context.Context, userID string) (*MFAChallenge, error)
	// Verify checks a submitted code against an issued challenge.
	Verify(ctx context.Context, challengeID, code string) (bool, error)
}

// MFAEnrollment is returned when a user begins TOTP enrollment: the shared
// secret and the otpauth:// URI to render as a QR code. The secret is shown
// exactly once, here.
type MFAEnrollment struct {
	Secret string
	URI    string
}

// MFAManager extends MFAProvider with the TOTP enrollment lifecycle.
type MFAManager interface {
	MFAProvider
	// BeginEnrollment generates and stores a pending secret and returns the
	// enrollment material to display.
	BeginEnrollment(ctx context.Context, userID, account string) (*MFAEnrollment, error)
	// ConfirmEnrollment verifies a code against the pending secret and, on
	// success, marks the user enrolled.
	ConfirmEnrollment(ctx context.Context, userID, code string) error
	// DisableMFA clears a user's enrollment.
	DisableMFA(ctx context.Context, userID string) error
}

// MFASecretStore persists per-user TOTP secrets and enrollment state.
type MFASecretStore interface {
	// GetMFA returns the user's secret and whether enrollment is confirmed.
	GetMFA(ctx context.Context, userID string) (secret string, enrolled bool, err error)
	SetMFASecret(ctx context.Context, userID, secret string) error
	MarkMFAEnrolled(ctx context.Context, userID string) error
	ClearMFA(ctx context.Context, userID string) error
}

// totpMFA is the production TOTP MFA provider, backed by a secret store.
type totpMFA struct {
	store  MFASecretStore
	issuer string
}

// NewTOTPMFA returns a TOTP-based MFA provider. issuer labels the otpauth URI
// (the name shown in authenticator apps).
func NewTOTPMFA(store MFASecretStore, issuer string) MFAManager {
	if issuer == "" {
		issuer = "Iris"
	}
	return &totpMFA{store: store, issuer: issuer}
}

func (m *totpMFA) Enrolled(ctx context.Context, userID string) (bool, error) {
	_, enrolled, err := m.store.GetMFA(ctx, userID)
	return enrolled, err
}

// Challenge for TOTP is stateless: the "challenge" is simply a prompt for the
// current code. The challenge id carries the user id for Verify.
func (m *totpMFA) Challenge(_ context.Context, userID string) (*MFAChallenge, error) {
	return &MFAChallenge{ChallengeID: userID, UserID: userID, Method: MFATOTP}, nil
}

func (m *totpMFA) Verify(ctx context.Context, challengeID, code string) (bool, error) {
	secret, enrolled, err := m.store.GetMFA(ctx, challengeID)
	if err != nil {
		return false, err
	}
	if !enrolled || secret == "" {
		return false, FailedPrecondition("MFA_NOT_ENROLLED", "user is not enrolled in MFA")
	}
	if !VerifyTOTP(secret, code, time.Now()) {
		return false, Invalid("MFA_CODE_INVALID", "invalid mfa code")
	}
	return true, nil
}

func (m *totpMFA) BeginEnrollment(ctx context.Context, userID, account string) (*MFAEnrollment, error) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		return nil, err
	}
	if err := m.store.SetMFASecret(ctx, userID, secret); err != nil {
		return nil, err
	}
	if account == "" {
		account = userID
	}
	return &MFAEnrollment{Secret: secret, URI: TOTPProvisioningURI(secret, account, m.issuer)}, nil
}

func (m *totpMFA) ConfirmEnrollment(ctx context.Context, userID, code string) error {
	secret, _, err := m.store.GetMFA(ctx, userID)
	if err != nil {
		return err
	}
	if secret == "" {
		return FailedPrecondition("MFA_NOT_STARTED", "begin enrollment before confirming")
	}
	if !VerifyTOTP(secret, code, time.Now()) {
		return Invalid("MFA_CODE_INVALID", "invalid mfa code")
	}
	return m.store.MarkMFAEnrolled(ctx, userID)
}

func (m *totpMFA) DisableMFA(ctx context.Context, userID string) error {
	return m.store.ClearMFA(ctx, userID)
}

// placeholderMFA is a development-only provider. It treats any non-empty code
// as valid and never reports enrollment, so production must inject a real one.
type placeholderMFA struct{}

// NewPlaceholderMFA returns a development MFA provider.
func NewPlaceholderMFA() MFAProvider { return placeholderMFA{} }

func (placeholderMFA) Enrolled(context.Context, string) (bool, error) { return false, nil }

func (placeholderMFA) Challenge(_ context.Context, userID string) (*MFAChallenge, error) {
	return &MFAChallenge{ChallengeID: "dev-challenge", UserID: userID, Method: MFATOTP}, nil
}

func (placeholderMFA) Verify(_ context.Context, _ string, code string) (bool, error) {
	if code == "" {
		return false, Invalid("MFA_CODE_REQUIRED", "mfa code required")
	}
	return true, nil
}
