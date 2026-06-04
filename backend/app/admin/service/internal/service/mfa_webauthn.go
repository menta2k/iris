package service

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// webAuthnEngine wraps the go-webauthn instance. nil when WebAuthn isn't
// configured (IRIS_WEBAUTHN_RP_ID unset) — passkey flows then return
// ErrMFAWebAuthnOff while TOTP/backup keep working.
type webAuthnEngine struct{ wa *webauthn.WebAuthn }

// newWebAuthnEngine builds the engine. rpID is the registrable domain (e.g.
// "kmx.example.com"); origins are the full URLs users hit (e.g.
// "https://kmx.example.com:8443").
func newWebAuthnEngine(rpID, rpDisplayName string, origins []string) (*webAuthnEngine, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpDisplayName,
		RPOrigins:     origins,
	})
	if err != nil {
		return nil, err
	}
	return &webAuthnEngine{wa: wa}, nil
}

// mfaWebAuthnUser adapts an iris user to the go-webauthn User interface.
type mfaWebAuthnUser struct {
	id       uint32
	username string
	creds    []webauthn.Credential
}

func (u *mfaWebAuthnUser) WebAuthnID() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(u.id))
	return b
}
func (u *mfaWebAuthnUser) WebAuthnName() string                   { return u.username }
func (u *mfaWebAuthnUser) WebAuthnDisplayName() string            { return u.username }
func (u *mfaWebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// webAuthnLoginSession is stashed in the session store between BeginLogin and
// the assertion: it links the ceremony back to the password-step challenge.
type webAuthnLoginSession struct {
	MFAToken string               `json:"mfa_token"`
	Session  webauthn.SessionData `json:"session"`
}

const (
	waEnrollPrefix = "wa_enroll:"
	waLoginPrefix  = "wa_login:"
)

// loadWebAuthnUser builds the go-webauthn user from the stored passkeys and a
// map of base64url(credID) → mfa_credentials row id (for sign-count updates).
func (s *MFAService) loadWebAuthnUser(ctx context.Context, userID uint32, username string) (*mfaWebAuthnUser, map[string]uint32, error) {
	rows, err := s.store.ListActive(ctx, userID, MFAKindWebAuthn)
	if err != nil {
		return nil, nil, err
	}
	creds := make([]webauthn.Credential, 0, len(rows))
	idMap := make(map[string]uint32, len(rows))
	for _, r := range rows {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(r.Secret), &c); err != nil {
			continue // skip a corrupt blob rather than fail the whole login
		}
		creds = append(creds, c)
		idMap[base64.RawURLEncoding.EncodeToString(c.ID)] = r.ID
	}
	return &mfaWebAuthnUser{id: userID, username: username, creds: creds}, idMap, nil
}

// EnrollPasskeyStart begins WebAuthn registration. Returns the JSON creation
// options for navigator.credentials.create and an operation id keyed to the
// stashed ceremony session.
func (s *MFAService) EnrollPasskeyStart(ctx context.Context, userID uint32, username string) (string, string, error) {
	if s.webauthn == nil {
		return "", "", ErrMFAWebAuthnOff
	}
	user, _, err := s.loadWebAuthnUser(ctx, userID, username)
	if err != nil {
		return "", "", err
	}
	options, sessionData, err := s.webauthn.wa.BeginRegistration(user)
	if err != nil {
		return "", "", fmt.Errorf("mfa: begin registration: %w", err)
	}
	opID, err := randomToken()
	if err != nil {
		return "", "", err
	}
	sd, err := json.Marshal(sessionData)
	if err != nil {
		return "", "", err
	}
	if err := s.sessions.Put(ctx, waEnrollPrefix+opID, string(sd), mfaEnrollTTL); err != nil {
		return "", "", err
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return "", "", err
	}
	return string(optionsJSON), opID, nil
}

// EnrollPasskeyFinish verifies the attestation and stores the new credential.
func (s *MFAService) EnrollPasskeyFinish(ctx context.Context, userID uint32, username, operationID, responseJSON, label string) error {
	if s.webauthn == nil {
		return ErrMFAWebAuthnOff
	}
	raw, ok, err := s.sessions.GetDel(ctx, waEnrollPrefix+operationID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrMFAEnrollExpired
	}
	var sessionData webauthn.SessionData
	if err := json.Unmarshal([]byte(raw), &sessionData); err != nil {
		return ErrMFAEnrollExpired
	}
	user, _, err := s.loadWebAuthnUser(ctx, userID, username)
	if err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(strings.NewReader(responseJSON))
	if err != nil {
		return fmt.Errorf("mfa: parse attestation: %w", err)
	}
	cred, err := s.webauthn.wa.CreateCredential(user, sessionData, parsed)
	if err != nil {
		return fmt.Errorf("mfa: create credential: %w", err)
	}
	blob, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	if strings.TrimSpace(label) == "" {
		label = "passkey"
	}
	now := s.now()
	_, err = s.store.Create(ctx, MFACredentialRow{
		UserID:      userID,
		Kind:        MFAKindWebAuthn,
		Secret:      string(blob),
		Label:       label,
		SignCount:   cred.Authenticator.SignCount,
		ConfirmedAt: &now,
	})
	return err
}

// RemovePasskey disables one of the user's passkeys.
func (s *MFAService) RemovePasskey(ctx context.Context, userID, id uint32) error {
	cred, err := s.store.GetActive(ctx, userID, id)
	if err != nil {
		return err
	}
	if cred == nil || cred.Kind != MFAKindWebAuthn {
		return errors.New("mfa: passkey not found")
	}
	return s.store.Disable(ctx, id)
}

// WebAuthnLoginStart begins a login ceremony for an MFA-challenged user.
func (s *MFAService) WebAuthnLoginStart(ctx context.Context, mfaToken string) (string, string, error) {
	if s.webauthn == nil {
		return "", "", ErrMFAWebAuthnOff
	}
	claims, err := s.jwt.VerifyMFAChallenge(mfaToken)
	if err != nil {
		return "", "", ErrMFAChallengeInvalid
	}
	user, _, err := s.loadWebAuthnUser(ctx, claims.UserID, claims.Username)
	if err != nil {
		return "", "", err
	}
	options, sessionData, err := s.webauthn.wa.BeginLogin(user)
	if err != nil {
		return "", "", fmt.Errorf("mfa: begin login: %w", err)
	}
	opID, err := randomToken()
	if err != nil {
		return "", "", err
	}
	payload, err := json.Marshal(webAuthnLoginSession{MFAToken: mfaToken, Session: *sessionData})
	if err != nil {
		return "", "", err
	}
	if err := s.sessions.Put(ctx, waLoginPrefix+opID, string(payload), mfaWebAuthnTTL); err != nil {
		return "", "", err
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return "", "", err
	}
	return string(optionsJSON), opID, nil
}

// WebAuthnLoginFinish validates the assertion and mints the real tokens.
func (s *MFAService) WebAuthnLoginFinish(ctx context.Context, operationID, responseJSON, clientIP string) (*LoginResponse, error) {
	if s.webauthn == nil {
		return nil, ErrMFAWebAuthnOff
	}
	raw, ok, err := s.sessions.GetDel(ctx, waLoginPrefix+operationID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrMFAChallengeInvalid
	}
	var ls webAuthnLoginSession
	if err := json.Unmarshal([]byte(raw), &ls); err != nil {
		return nil, ErrMFAChallengeInvalid
	}
	claims, err := s.jwt.VerifyMFAChallenge(ls.MFAToken)
	if err != nil {
		return nil, ErrMFAChallengeInvalid
	}
	user, idMap, err := s.loadWebAuthnUser(ctx, claims.UserID, claims.Username)
	if err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(strings.NewReader(responseJSON))
	if err != nil {
		return nil, fmt.Errorf("mfa: parse assertion: %w", err)
	}
	cred, err := s.webauthn.wa.ValidateLogin(user, ls.Session, parsed)
	if err != nil {
		return nil, ErrMFAInvalidCode
	}
	// Persist updated clone-detection state.
	if rowID, ok := idMap[base64.RawURLEncoding.EncodeToString(cred.ID)]; ok {
		if blob, mErr := json.Marshal(cred); mErr == nil {
			_ = s.store.UpdateSecret(ctx, rowID, string(blob), cred.Authenticator.SignCount)
		}
	}
	return s.issueTokens(ctx, claims.UserID, claims.Username, claims.Roles, clientIP)
}
