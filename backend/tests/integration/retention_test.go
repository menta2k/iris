package integration

import (
	"context"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestRetentionDropsOldChunksAndReclaimsDisk proves the core claim: dropping old
// TimescaleDB chunks removes the data and returns disk to the OS. It inserts rows
// into two daily chunks (100 days old and 1 day old), runs a 90-day retention,
// and asserts the old chunk was dropped, its rows are gone, and the hypertable
// shrank. Skips when TimescaleDB is absent (plain PostgreSQL has no chunks).
func TestRetentionDropsOldChunksAndReclaimsDisk(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	var isHyper bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb')
		   AND EXISTS (SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'mail_records')
	`).Scan(&isHyper); err != nil || !isHyper {
		t.Skip("mail_records is not a TimescaleDB hypertable; skipping chunk retention test")
	}

	// Two distinct daily chunks: an old one (eligible for drop) and a recent one.
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO mail_records (message_id, event_time, status)
		SELECT 'old-' || g, now() - interval '100 days', 'sent' FROM generate_series(1, 3000) g`); err != nil {
		t.Fatalf("insert old rows: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO mail_records (message_id, event_time, status)
		SELECT 'new-' || g, now() - interval '1 day', 'sent' FROM generate_series(1, 200) g`); err != nil {
		t.Fatalf("insert recent rows: %v", err)
	}

	var sizeBefore int64
	_ = db.Pool.QueryRow(ctx, `SELECT hypertable_size('mail_records'::regclass)`).Scan(&sizeBefore)

	repo := data.NewRetentionRepo(db)
	table, _ := biz.ManagedTableByName("mail_records")
	run, err := repo.RunRetention(ctx, &biz.RetentionPolicy{TableName: "mail_records", RetentionDays: 90}, table)
	if err != nil {
		t.Fatalf("run retention: %v", err)
	}
	if run.Error != "" {
		t.Fatalf("retention reported error: %s", run.Error)
	}
	if run.ChunksDropped < 1 {
		t.Fatalf("expected at least one chunk dropped, got %d", run.ChunksDropped)
	}

	// The old rows are gone; the recent rows remain.
	var oldCount, newCount int
	_ = db.Pool.QueryRow(ctx, `SELECT count(*) FROM mail_records WHERE event_time < now() - interval '95 days'`).Scan(&oldCount)
	_ = db.Pool.QueryRow(ctx, `SELECT count(*) FROM mail_records`).Scan(&newCount)
	if oldCount != 0 {
		t.Fatalf("expected old rows dropped, %d remain", oldCount)
	}
	if newCount != 200 {
		t.Fatalf("expected 200 recent rows to survive, got %d", newCount)
	}

	// Disk was reclaimed: the run recorded a smaller after-size, and the live size
	// shrank versus before the drop.
	if run.BytesAfter >= run.BytesBefore {
		t.Fatalf("expected bytes_after (%d) < bytes_before (%d)", run.BytesAfter, run.BytesBefore)
	}
	var sizeAfter int64
	_ = db.Pool.QueryRow(ctx, `SELECT hypertable_size('mail_records'::regclass)`).Scan(&sizeAfter)
	if sizeAfter >= sizeBefore {
		t.Fatalf("expected hypertable to shrink: before=%d after=%d", sizeBefore, sizeAfter)
	}
}
