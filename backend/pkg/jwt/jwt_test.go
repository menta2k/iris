package jwt

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestIssuer(t *testing.T) *Issuer {
	t.Helper()
	access := []byte(strings.Repeat("a", 32))
	refresh := []byte(strings.Repeat("b", 32))
	iss, err := NewIssuer(Config{
		AccessSecret:  access,
		RefreshSecret: refresh,
		AccessTTL:     time.Minute,
		RefreshTTL:    time.Hour,
		Issuer:        "iris",
		Audience:      []string{"kumo-ui-admin"},
		KeyID:         "k1",
	})
	require.NoError(t, err)
	return iss
}

func TestIssueAndVerifyAccess(t *testing.T) {
	iss := newTestIssuer(t)
	now := time.Now()

	token, exp, err := iss.IssueAccess(42, "alice", []string{"admin"}, now)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.WithinDuration(t, now.Add(time.Minute), exp, time.Second)

	claims, err := iss.VerifyAccess(token)
	require.NoError(t, err)
	require.Equal(t, uint32(42), claims.UserID)
	require.Equal(t, "alice", claims.Username)
	require.Equal(t, tokenTypeAccess, claims.TokenType)
	require.Equal(t, []string{"admin"}, claims.Roles)
}

func TestVerifyRejectsWrongType(t *testing.T) {
	iss := newTestIssuer(t)
	now := time.Now()

	access, _, err := iss.IssueAccess(1, "u", nil, now)
	require.NoError(t, err)
	_, err = iss.VerifyRefresh(access)
	require.Error(t, err)
}

func TestVerifyRejectsExpired(t *testing.T) {
	iss := newTestIssuer(t)
	old := time.Now().Add(-2 * time.Hour)
	token, _, err := iss.IssueAccess(1, "u", nil, old)
	require.NoError(t, err)
	_, err = iss.VerifyAccess(token)
	require.ErrorIs(t, err, ErrTokenExpired)
}

func TestNewIssuerRejectsShortSecret(t *testing.T) {
	_, err := NewIssuer(Config{
		AccessSecret:  []byte("short"),
		RefreshSecret: []byte(strings.Repeat("b", 32)),
	})
	require.ErrorIs(t, err, ErrSecretTooShort)
}

func TestVerifyAccessRejectsTampered(t *testing.T) {
	iss := newTestIssuer(t)
	tok, _, err := iss.IssueAccess(1, "u", nil, time.Now())
	require.NoError(t, err)
	tampered := tok[:len(tok)-2] + "AA"
	_, err = iss.VerifyAccess(tampered)
	require.Error(t, err)
}

func TestRefreshTokenRoundTrip(t *testing.T) {
	iss := newTestIssuer(t)
	tok, _, err := iss.IssueRefresh(7, "bob", time.Now())
	require.NoError(t, err)
	claims, err := iss.VerifyRefresh(tok)
	require.NoError(t, err)
	require.Equal(t, uint32(7), claims.UserID)
	require.Equal(t, tokenTypeRefresh, claims.TokenType)
}

func TestNewIssuerAppliesDefaults(t *testing.T) {
	iss, err := NewIssuer(Config{
		AccessSecret:  []byte(strings.Repeat("a", 32)),
		RefreshSecret: []byte(strings.Repeat("b", 32)),
	})
	require.NoError(t, err)
	require.Equal(t, time.Hour, iss.accessTTL)
	require.Equal(t, 7*24*time.Hour, iss.refreshTTL)
	require.Equal(t, "iris", iss.issuer)
	require.Equal(t, "default", iss.keyID)
}

func TestVerifyRejectsAlgNone(t *testing.T) {
	iss := newTestIssuer(t)
	// Hand-crafted "alg=none" token; should be refused by valid-methods filter.
	header := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0"
	payload := "eyJ1aWQiOjEsInR0IjoiYWNjZXNzIn0"
	noneToken := header + "." + payload + "."
	_, err := iss.VerifyAccess(noneToken)
	require.Error(t, err)
}
