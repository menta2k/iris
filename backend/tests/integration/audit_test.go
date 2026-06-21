package integration

import (
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestAuditAppendOnly verifies audit entries accumulate, are returned
// newest-first, and that the writer redacts sensitive change-summary fields.
func TestAuditAppendOnly(t *testing.T) {
	db := setupDB(t)
	repo := data.NewAuditRepo(db)
	auditor := biz.NewAuditor(repo)
	ctx := ownerCtx()

	if err := auditor.Record(ctx, "vmta.create", "vmta", "v1", biz.AuditSuccess, map[string]any{
		"name": "vmta-1", "secret_ref": "should-be-redacted",
	}); err != nil {
		t.Fatalf("record 1: %v", err)
	}
	if err := auditor.Record(ctx, "vmta.delete", "vmta", "v1", biz.AuditDenied, map[string]any{
		"name": "vmta-1",
	}); err != nil {
		t.Fatalf("record 2: %v", err)
	}

	entries, err := repo.List(ctx, biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 audit entries, got %d", len(entries))
	}
	// Newest first.
	if entries[0].Operation != "vmta.delete" || entries[1].Operation != "vmta.create" {
		t.Fatalf("expected newest-first ordering, got %s then %s", entries[0].Operation, entries[1].Operation)
	}
	// Redaction persisted.
	if entries[1].SafeChangeSummary["secret_ref"] != "[REDACTED]" {
		t.Fatalf("expected secret_ref redacted, got %v", entries[1].SafeChangeSummary["secret_ref"])
	}
	if entries[0].Outcome != "denied" {
		t.Fatalf("expected denied outcome recorded, got %q", entries[0].Outcome)
	}
}
