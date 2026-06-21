package biz

import (
	"context"
	"testing"
)

// captureWriter records audit events for assertion.
type captureWriter struct{ events []AuditEvent }

func (c *captureWriter) Write(_ context.Context, e AuditEvent) error {
	c.events = append(c.events, e)
	return nil
}

func TestAuditorRedactsAndAttributesActor(t *testing.T) {
	w := &captureWriter{}
	auditor := NewAuditor(w)
	ctx := WithIdentity(context.Background(), &Identity{
		UserID: "u1", IPAddress: "10.0.0.5", RequestID: "req-1",
	})
	err := auditor.Record(ctx, "dkim.create", "dkim", "d1", AuditSuccess, map[string]any{
		"domain":          "example.com",
		"private_key_ref": "super-secret-value",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(w.events))
	}
	e := w.events[0]
	if e.ActorUserID != "u1" || e.IPAddress != "10.0.0.5" || e.RequestID != "req-1" {
		t.Fatalf("actor metadata not attributed: %+v", e)
	}
	if e.SafeChangeSummary["private_key_ref"] != "[REDACTED]" {
		t.Fatalf("sensitive field not redacted: %v", e.SafeChangeSummary["private_key_ref"])
	}
	if e.SafeChangeSummary["domain"] != "example.com" {
		t.Fatalf("non-sensitive field should be preserved")
	}
}

func TestIsSensitiveKey(t *testing.T) {
	for _, k := range []string{"password", "Session_Token", "dkim_private_key", "secret_ref"} {
		if !IsSensitiveKey(k) {
			t.Fatalf("expected %q to be sensitive", k)
		}
	}
	for _, k := range []string{"email", "domain", "name", "status"} {
		if IsSensitiveKey(k) {
			t.Fatalf("expected %q not to be sensitive", k)
		}
	}
}

func TestAuditEntrySafeSummary(t *testing.T) {
	e := AuditEntry{SafeChangeSummary: map[string]any{"token": "abc", "name": "ok"}}
	s := e.SafeSummary()
	if s["token"] != "[REDACTED]" || s["name"] != "ok" {
		t.Fatalf("unexpected safe summary: %v", s)
	}
}
