package integration

import (
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestRspamdResultPersistence verifies that Rspamd filter results persist and
// list newest-first.
func TestRspamdResultPersistence(t *testing.T) {
	db := setupDB(t)
	repo := data.NewInboundRepo(db)
	uc := biz.NewInboundUsecase(repo, nil, true)
	ctx := ownerCtx()

	if err := uc.IngestRspamdResult(ctx, &biz.RspamdFilterResult{
		Action: biz.RspamdReject, Score: 12.5, Symbols: []string{"BAYES_SPAM"}, Reason: "high score",
	}); err != nil {
		t.Fatalf("ingest rspamd: %v", err)
	}
	results, err := uc.ListRspamdResults(ctx, biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list rspamd: %v", err)
	}
	if len(results) != 1 || results[0].Action != biz.RspamdReject || results[0].Score != 12.5 {
		t.Fatalf("expected one reject result, got %+v", results)
	}
}
