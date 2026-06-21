package biz

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// SessionClaims is the validated payload of a session token.
type SessionClaims struct {
	UserID      string    `json:"uid"`
	Email       string    `json:"eml"`
	MFAVerified bool      `json:"mfa"`
	ExpiresAt   time.Time `json:"-"`
}

// sessionPayload is the wire form (compact field names, unix expiry).
type sessionPayload struct {
	UserID      string `json:"uid"`
	Email       string `json:"eml"`
	MFAVerified bool   `json:"mfa"`
	Expiry      int64  `json:"exp"`
}

// SessionManager issues and validates stateless, HMAC-signed session tokens.
// Tokens are self-contained (`<base64url(payload)>.<base64url(sig)>`) and carry
// whether the session has cleared MFA, so a partially-authenticated token can
// be upgraded after VerifyMFA. The signing secret is the deployment's
// session_token_secret; rotating it invalidates all outstanding tokens.
type SessionManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewSessionManager constructs a manager. ttl bounds token lifetime; a
// non-positive ttl falls back to 12h.
func NewSessionManager(secret string, ttl time.Duration) *SessionManager {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &SessionManager{secret: []byte(secret), ttl: ttl, now: time.Now}
}

// Issue mints a signed token for the user with the given MFA state, expiring
// after the configured TTL.
func (m *SessionManager) Issue(userID, email string, mfaVerified bool) (string, error) {
	payload := sessionPayload{
		UserID:      userID,
		Email:       email,
		MFAVerified: mfaVerified,
		Expiry:      m.now().Add(m.ttl).Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", Internal(err, "marshal session")
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	sig := base64.RawURLEncoding.EncodeToString(m.sign([]byte(body)))
	return body + "." + sig, nil
}

// Parse validates the signature and expiry and returns the claims. Any
// tampering, malformed token, or expiry yields an Unauthorized error.
func (m *SessionManager) Parse(token string) (*SessionClaims, error) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok || body == "" || sig == "" {
		return nil, Unauthorized("INVALID_TOKEN", "malformed session token")
	}
	wantSig, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return nil, Unauthorized("INVALID_TOKEN", "malformed session token")
	}
	if subtle.ConstantTimeCompare(wantSig, m.sign([]byte(body))) != 1 {
		return nil, Unauthorized("INVALID_TOKEN", "invalid session token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, Unauthorized("INVALID_TOKEN", "malformed session token")
	}
	var payload sessionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, Unauthorized("INVALID_TOKEN", "malformed session token")
	}
	exp := time.Unix(payload.Expiry, 0)
	if !m.now().Before(exp) {
		return nil, Unauthorized("SESSION_EXPIRED", "session has expired")
	}
	return &SessionClaims{
		UserID:      payload.UserID,
		Email:       payload.Email,
		MFAVerified: payload.MFAVerified,
		ExpiresAt:   exp,
	}, nil
}

func (m *SessionManager) sign(body []byte) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write(body)
	return mac.Sum(nil)
}
