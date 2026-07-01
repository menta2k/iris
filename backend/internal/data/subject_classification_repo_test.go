package data

import (
	"context"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
)

func TestSubjectClassificationRepo(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `TRUNCATE subject_classifications`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	repo := NewSubjectClassificationRepo(db)

	// Seed a manual rule.
	created, err := repo.Create(ctx, &biz.SubjectClassification{
		Subject:           "Your order has shipped",
		SubjectNormalized: biz.NormalizeSubject("Your order has shipped"),
		Label:             "shipping update",
		Source:            biz.ClassificationSourceManual,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// A near-identical subject (different order number) matches via trigram.
	norm := biz.NormalizeSubject("Your order #55123 has shipped")
	m, err := repo.BestMatch(ctx, norm, 0.45)
	if err != nil {
		t.Fatalf("best match: %v", err)
	}
	if m == nil || m.Label != "shipping update" {
		t.Fatalf("expected trigram match to the shipping rule, got %+v", m)
	}

	// An unrelated subject does not match at the same threshold.
	if m, err := repo.BestMatch(ctx, biz.NormalizeSubject("Password reset requested"), 0.45); err != nil {
		t.Fatalf("best match 2: %v", err)
	} else if m != nil {
		t.Fatalf("unrelated subject should not match, got %+v", m)
	}

	// Upsert an AI label; a second upsert of the same normalized key updates it,
	// not duplicates.
	up1, err := repo.Upsert(ctx, &biz.SubjectClassification{
		Subject: "Invoice 900", SubjectNormalized: biz.NormalizeSubject("Invoice 900"),
		Label: "invoice", Source: biz.ClassificationSourceAI,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	up2, err := repo.Upsert(ctx, &biz.SubjectClassification{
		Subject: "Invoice 900", SubjectNormalized: biz.NormalizeSubject("Invoice 900"),
		Label: "billing", Source: biz.ClassificationSourceAI,
	})
	if err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	if up1.ID != up2.ID {
		t.Errorf("upsert on same key should update in place: %s vs %s", up1.ID, up2.ID)
	}
	if up2.Label != "billing" {
		t.Errorf("upsert should update label, got %q", up2.Label)
	}

	// IncrementHit + List.
	if err := repo.IncrementHit(ctx, created.ID); err != nil {
		t.Fatalf("increment: %v", err)
	}
	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("list len = %d, want 2", len(all))
	}

	// Delete + not-found on re-delete.
	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.Delete(ctx, created.ID); err == nil {
		t.Error("re-deleting a missing rule should error")
	}
}
