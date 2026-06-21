package biz

import (
	"testing"
	"time"
)

func TestSessionIssueAndParse(t *testing.T) {
	m := NewSessionManager("secret-key", time.Hour)
	tok, err := m.Issue("user-1", "u@example.com", true)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := m.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != "user-1" || claims.Email != "u@example.com" || !claims.MFAVerified {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestSessionRejectsTamperedToken(t *testing.T) {
	m := NewSessionManager("secret-key", time.Hour)
	tok, _ := m.Issue("user-1", "u@example.com", false)
	for _, bad := range []string{tok + "x", "garbage", "", "a.b", tok[:len(tok)-2]} {
		if _, err := m.Parse(bad); err == nil {
			t.Fatalf("expected rejection for %q", bad)
		}
	}
}

func TestSessionRejectsWrongSecret(t *testing.T) {
	a := NewSessionManager("secret-a", time.Hour)
	b := NewSessionManager("secret-b", time.Hour)
	tok, _ := a.Issue("u", "u@example.com", true)
	if _, err := b.Parse(tok); err == nil {
		t.Fatal("token signed with a different secret must not verify")
	}
}

func TestSessionExpiry(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewSessionManager("secret-key", time.Minute)
	m.now = func() time.Time { return base }
	tok, _ := m.Issue("u", "u@example.com", true)

	// Still valid 59s later.
	m.now = func() time.Time { return base.Add(59 * time.Second) }
	if _, err := m.Parse(tok); err != nil {
		t.Fatalf("token should still be valid: %v", err)
	}
	// Expired 61s later.
	m.now = func() time.Time { return base.Add(61 * time.Second) }
	if _, err := m.Parse(tok); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}
