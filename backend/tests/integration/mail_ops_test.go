package integration

import (
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestQueueSnapshotUpsertAndList verifies queue state upsert and listing.
func TestQueueSnapshotUpsertAndList(t *testing.T) {
	db := setupDB(t)
	repo := data.NewMailOpsRepo(db)
	ctx := ownerCtx()

	if err := repo.UpsertQueueState(ctx, &biz.MailclassQueue{
		Mailclass: "bulk", State: biz.QueueRunning, Depth: 5, OldestMessageAgeSeconds: 30,
	}); err != nil {
		t.Fatalf("upsert queue: %v", err)
	}
	// Upsert again with new depth; the current row should update, not duplicate.
	if err := repo.UpsertQueueState(ctx, &biz.MailclassQueue{
		Mailclass: "bulk", State: biz.QueuePaused, Depth: 12,
	}); err != nil {
		t.Fatalf("re-upsert queue: %v", err)
	}

	queues, err := repo.ListQueues(ctx, biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list queues: %v", err)
	}
	if len(queues) != 1 {
		t.Fatalf("expected one queue row, got %d", len(queues))
	}
	if queues[0].State != biz.QueuePaused || queues[0].Depth != 12 {
		t.Fatalf("expected updated state, got %+v", queues[0])
	}
}

// TestServiceControlSingleActive verifies the active-request invariant and the
// lifecycle transition recorded in the database.
func TestServiceControlSingleActive(t *testing.T) {
	db := setupDB(t)
	repo := data.NewMailOpsRepo(db)
	ctx := ownerCtx()

	rec, err := repo.CreateServiceControlRequest(ctx, &biz.ServiceControlRecord{Operation: "reload", ConfirmationID: "c1"})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	active, err := repo.ActiveServiceControlExists(ctx)
	if err != nil || !active {
		t.Fatalf("expected an active request, got active=%v err=%v", active, err)
	}
	if err := repo.UpdateServiceControlStatus(ctx, rec.ID, biz.SvcSucceeded, "done"); err != nil {
		t.Fatalf("update status: %v", err)
	}
	active, err = repo.ActiveServiceControlExists(ctx)
	if err != nil || active {
		t.Fatalf("expected no active request after completion, got active=%v err=%v", active, err)
	}
}
