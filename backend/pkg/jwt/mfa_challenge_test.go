package jwt

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func testIssuer(t *testing.T) *Issuer {
	t.Helper()
	iss, err := NewIssuer(Config{
		AccessSecret:  []byte(strings.Repeat("a", 32)),
		RefreshSecret: []byte(strings.Repeat("b", 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return iss
}

func TestMFAChallengeRoundTrip(t *testing.T) {
	iss := testIssuer(t)
	tok, _, err := iss.IssueMFAChallenge(7, "alice", []string{"admin"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	claims, err := iss.VerifyMFAChallenge(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.UserID != 7 || claims.Username != "alice" {
		t.Fatalf("claims = %+v", claims)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Fatalf("roles = %v", claims.Roles)
	}
}

func TestMFAChallengeNotUsableAsAccess(t *testing.T) {
	iss := testIssuer(t)
	tok, _, _ := iss.IssueMFAChallenge(7, "alice", nil, time.Now())
	// A challenge token must be rejected by the access verifier (wrong type).
	if _, err := iss.VerifyAccess(tok); !errors.Is(err, ErrTokenWrongType) {
		t.Fatalf("expected ErrTokenWrongType, got %v", err)
	}
}

func TestMFAChallengeExpired(t *testing.T) {
	iss := testIssuer(t)
	// Issue as of 10 minutes ago → past the 5-minute TTL.
	tok, _, _ := iss.IssueMFAChallenge(7, "alice", nil, time.Now().Add(-10*time.Minute))
	if _, err := iss.VerifyMFAChallenge(tok); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}
