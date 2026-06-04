// Package service: MFAService implements optional, self-service multi-factor
// authentication (TOTP + WebAuthn + single-use backup codes), modelled on
// go-tangra-portal. Login is two-step: AuthenticationService verifies the
// password and, when the user has an active second factor, returns a
// short-lived MFA-challenge JWT instead of tokens; the client then calls a
// verify endpoint that this service handles to mint the real tokens.
package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"

	appcrypto "github.com/menta2k/iris/backend/pkg/crypto"
	appjwt "github.com/menta2k/iris/backend/pkg/jwt"
)

const (
	totpIssuer        = "Iris"
	backupCodeCount   = 10
	mfaEnrollTTL      = 10 * time.Minute
	mfaWebAuthnTTL    = 5 * time.Minute
	totpEnrollPrefix  = "totp_enroll:"
	bcryptDefaultCost = 12
)

// Errors surfaced to the API layer.
var (
	ErrMFANotConfigured   = errors.New("mfa: not configured (set IRIS_MFA_SECRET_KEY)")
	ErrMFAWebAuthnOff      = errors.New("mfa: webauthn not configured")
	ErrMFAInvalidCode      = errors.New("mfa: invalid code")
	ErrMFAChallengeInvalid = errors.New("mfa: challenge invalid or expired")
	ErrMFAEnrollExpired    = errors.New("mfa: enrollment expired, restart")
)

// LoginSuccessRecorder lets the MFA verify path clear the failed-login
// counter / stamp last-login, mirroring the password path. Satisfied by the
// user repo.
type LoginSuccessRecorder interface {
	RecordLoginSuccess(ctx context.Context, userID uint32, ip string, at time.Time) error
}

// MFAService owns all MFA flows.
type MFAService struct {
	store     MFAStore
	sessions  MFASessionStore
	jwt       *appjwt.Issuer
	recorder  LoginSuccessRecorder
	aesKey    []byte         // nil when IRIS_MFA_SECRET_KEY unset → TOTP disabled
	webauthn  *webAuthnEngine // nil when WebAuthn unconfigured (see mfa_webauthn.go)
	bcryptCost int
	now       func() time.Time
}

// NewMFAServiceFromConfig builds the service and its WebAuthn engine from
// primitive config so a provider in another package can construct it without
// touching the unexported engine type. WebAuthn is enabled only when rpID and
// at least one origin are supplied.
func NewMFAServiceFromConfig(store MFAStore, sessions MFASessionStore, issuer *appjwt.Issuer, recorder LoginSuccessRecorder, aesKey []byte, rpID, rpDisplayName string, rpOrigins []string, bcryptCost int) (*MFAService, error) {
	var wae *webAuthnEngine
	if rpID != "" && len(rpOrigins) > 0 {
		e, err := newWebAuthnEngine(rpID, rpDisplayName, rpOrigins)
		if err != nil {
			return nil, fmt.Errorf("mfa: webauthn config: %w", err)
		}
		wae = e
	}
	return NewMFAService(store, sessions, issuer, recorder, aesKey, wae, bcryptCost), nil
}

// NewMFAService constructs the service. aesKey may be nil (TOTP enrollment
// then errors with ErrMFANotConfigured); webauthn may be nil.
func NewMFAService(store MFAStore, sessions MFASessionStore, issuer *appjwt.Issuer, recorder LoginSuccessRecorder, aesKey []byte, wae *webAuthnEngine, bcryptCost int) *MFAService {
	if bcryptCost <= 0 {
		bcryptCost = bcryptDefaultCost
	}
	return &MFAService{
		store: store, sessions: sessions, jwt: issuer, recorder: recorder,
		aesKey: aesKey, webauthn: wae, bcryptCost: bcryptCost, now: time.Now,
	}
}

// --- status -----------------------------------------------------------------

type MFAStatus struct {
	TOTPEnabled     bool
	Passkeys        []MFAPasskey
	BackupRemaining int
	WebAuthnEnabled bool // whether the server supports passkey enrollment
}

type MFAPasskey struct {
	ID        uint32
	Label     string
	CreatedAt time.Time
}

func (s *MFAService) Status(ctx context.Context, userID uint32) (*MFAStatus, error) {
	totpN, err := s.store.CountActive(ctx, userID, MFAKindTOTP)
	if err != nil {
		return nil, err
	}
	backupN, err := s.store.CountActive(ctx, userID, MFAKindBackup)
	if err != nil {
		return nil, err
	}
	passkeyRows, err := s.store.ListActive(ctx, userID, MFAKindWebAuthn)
	if err != nil {
		return nil, err
	}
	passkeys := make([]MFAPasskey, 0, len(passkeyRows))
	for _, p := range passkeyRows {
		passkeys = append(passkeys, MFAPasskey{ID: p.ID, Label: p.Label, CreatedAt: p.CreatedAt})
	}
	return &MFAStatus{
		TOTPEnabled:     totpN > 0,
		Passkeys:        passkeys,
		BackupRemaining: backupN,
		WebAuthnEnabled: s.webauthn != nil,
	}, nil
}

// --- TOTP enrollment ---------------------------------------------------------

type TOTPEnrollStart struct {
	Secret        string
	OTPAuthURL    string
	QRCodeDataURI string
	OperationID   string
}

// EnrollTOTPStart generates a fresh secret + QR and stashes the secret in the
// session store keyed by a returned operation id. Nothing is persisted until
// EnrollTOTPConfirm verifies a code, so an abandoned enrollment leaves no
// state.
func (s *MFAService) EnrollTOTPStart(ctx context.Context, userID uint32, username string) (*TOTPEnrollStart, error) {
	if len(s.aesKey) == 0 {
		return nil, ErrMFANotConfigured
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: username,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return nil, fmt.Errorf("mfa: totp generate: %w", err)
	}
	png, err := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("mfa: qr encode: %w", err)
	}
	opID, err := randomToken()
	if err != nil {
		return nil, err
	}
	if err := s.sessions.Put(ctx, totpEnrollPrefix+opID, fmt.Sprintf("%d:%s", userID, key.Secret()), mfaEnrollTTL); err != nil {
		return nil, fmt.Errorf("mfa: stash enroll: %w", err)
	}
	return &TOTPEnrollStart{
		Secret:        key.Secret(),
		OTPAuthURL:    key.URL(),
		QRCodeDataURI: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		OperationID:   opID,
	}, nil
}

// EnrollTOTPConfirm verifies the first code against the pending secret, then
// persists it (encrypted) and (re)issues a fresh set of backup codes, which
// are returned ONCE.
func (s *MFAService) EnrollTOTPConfirm(ctx context.Context, userID uint32, operationID, code string) ([]string, error) {
	if len(s.aesKey) == 0 {
		return nil, ErrMFANotConfigured
	}
	stored, ok, err := s.sessions.GetDel(ctx, totpEnrollPrefix+operationID)
	if err != nil {
		return nil, fmt.Errorf("mfa: read enroll: %w", err)
	}
	if !ok {
		return nil, ErrMFAEnrollExpired
	}
	var sessUID uint32
	var secret string
	if _, err := fmt.Sscanf(stored, "%d:", &sessUID); err != nil || sessUID != userID {
		return nil, ErrMFAEnrollExpired
	}
	secret = stored[indexByte(stored, ':')+1:]

	if !validateTOTP(code, secret) {
		return nil, ErrMFAInvalidCode
	}
	enc, err := appcrypto.EncryptSecret(s.aesKey, secret)
	if err != nil {
		return nil, fmt.Errorf("mfa: encrypt secret: %w", err)
	}
	// Replace any prior TOTP, then persist the confirmed one.
	if err := s.store.DisableByKind(ctx, userID, MFAKindTOTP); err != nil {
		return nil, err
	}
	now := s.now()
	if _, err := s.store.Create(ctx, MFACredentialRow{
		UserID: userID, Kind: MFAKindTOTP, Secret: enc, ConfirmedAt: &now,
	}); err != nil {
		return nil, err
	}
	return s.regenerateBackupCodes(ctx, userID)
}

// --- backup codes ------------------------------------------------------------

// RegenerateBackupCodes replaces the user's backup-code set and returns the
// new plaintext codes once.
func (s *MFAService) RegenerateBackupCodes(ctx context.Context, userID uint32) ([]string, error) {
	return s.regenerateBackupCodes(ctx, userID)
}

func (s *MFAService) regenerateBackupCodes(ctx context.Context, userID uint32) ([]string, error) {
	if err := s.store.DisableByKind(ctx, userID, MFAKindBackup); err != nil {
		return nil, err
	}
	codes := make([]string, 0, backupCodeCount)
	for i := 0; i < backupCodeCount; i++ {
		code, err := generateBackupCode()
		if err != nil {
			return nil, err
		}
		hash, err := appcrypto.HashPassword(code, s.bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("mfa: hash backup code: %w", err)
		}
		if _, err := s.store.Create(ctx, MFACredentialRow{
			UserID: userID, Kind: MFAKindBackup, Secret: hash,
		}); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

// --- disable / reset ---------------------------------------------------------

// Disable turns off all of a user's MFA (self-service).
func (s *MFAService) Disable(ctx context.Context, userID uint32) error {
	return s.store.DisableAll(ctx, userID)
}

// AdminReset is Disable invoked by an administrator on another account.
func (s *MFAService) AdminReset(ctx context.Context, userID uint32) error {
	return s.store.DisableAll(ctx, userID)
}

// --- login verification (TOTP / backup) -------------------------------------

// VerifyChallenge validates an MFA-challenge token plus a TOTP or backup
// code, and on success mints the real access/refresh tokens. Exactly one of
// totpCode / backupCode should be non-empty.
func (s *MFAService) VerifyChallenge(ctx context.Context, mfaToken, totpCode, backupCode, clientIP string) (*LoginResponse, error) {
	claims, err := s.jwt.VerifyMFAChallenge(mfaToken)
	if err != nil {
		return nil, ErrMFAChallengeInvalid
	}
	uid := claims.UserID

	var verified bool
	switch {
	case totpCode != "":
		verified, err = s.verifyTOTP(ctx, uid, totpCode)
	case backupCode != "":
		verified, err = s.verifyBackup(ctx, uid, backupCode)
	default:
		return nil, ErrMFAInvalidCode
	}
	if err != nil {
		return nil, err
	}
	if !verified {
		return nil, ErrMFAInvalidCode
	}
	return s.issueTokens(ctx, uid, claims.Username, claims.Roles, clientIP)
}

func (s *MFAService) verifyTOTP(ctx context.Context, userID uint32, code string) (bool, error) {
	cred, err := s.store.GetActiveTOTP(ctx, userID)
	if err != nil || cred == nil {
		return false, err
	}
	if len(s.aesKey) == 0 {
		return false, ErrMFANotConfigured
	}
	secret, err := appcrypto.DecryptSecret(s.aesKey, cred.Secret)
	if err != nil {
		return false, fmt.Errorf("mfa: decrypt secret: %w", err)
	}
	return validateTOTP(code, secret), nil
}

func (s *MFAService) verifyBackup(ctx context.Context, userID uint32, code string) (bool, error) {
	rows, err := s.store.ListActive(ctx, userID, MFAKindBackup)
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		if appcrypto.VerifyPassword(r.Secret, code) == nil {
			// Single-use: consume on match.
			if err := s.store.MarkUsed(ctx, r.ID); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// issueTokens mints the final token pair and records the successful login.
func (s *MFAService) issueTokens(ctx context.Context, userID uint32, username string, roles []string, clientIP string) (*LoginResponse, error) {
	now := s.now()
	access, accessExp, err := s.jwt.IssueAccess(userID, username, roles, now)
	if err != nil {
		return nil, fmt.Errorf("mfa: issue access: %w", err)
	}
	refresh, _, err := s.jwt.IssueRefresh(userID, username, now)
	if err != nil {
		return nil, fmt.Errorf("mfa: issue refresh: %w", err)
	}
	if s.recorder != nil {
		_ = s.recorder.RecordLoginSuccess(ctx, userID, clientIP, now)
	}
	return &LoginResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(time.Until(accessExp).Seconds()),
		UserID:       userID,
		Username:     username,
		Roles:        roles,
	}, nil
}

// --- helpers -----------------------------------------------------------------

func validateTOTP(code, secret string) bool {
	ok, err := totp.ValidateCustom(code, secret, time.Now().UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      1, // ±30s clock drift tolerance
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// generateBackupCode returns a human-friendly single-use code (8 hex chars,
// dash-grouped). 32 bits of entropy per code; bcrypt-hashed at rest.
func generateBackupCode() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("mfa: backup rand: %w", err)
	}
	h := hex.EncodeToString(b)
	return h[:4] + "-" + h[4:], nil
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("mfa: token rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
