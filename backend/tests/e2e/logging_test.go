//go:build e2e

package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/worker"
)

// TestLoggingToMailRecords proves the full logging pipeline through a real
// kumod: the generated policy's log_hook XADDs KumoMTA's structured records onto
// Redis, the LogStreamWorker ingests them, and a Reception + Delivery for the
// injected message land in mail_records (the table that powers the Logs UI).
//
// kumod (in its container) XADDs to redis://iris-redis:6379 on the rig network;
// the worker here reads the same Redis via the host-published port. Requires the
// dev DB/Redis (IRIS_TEST_DSN / IRIS_TEST_REDIS), like the integration suite.
func TestLoggingToMailRecords(t *testing.T) {
	requireE2E(t)
	requireDocker(t)
	dsn := os.Getenv("IRIS_TEST_DSN")
	redisAddr := os.Getenv("IRIS_TEST_REDIS")
	if dsn == "" || redisAddr == "" {
		t.Skip("IRIS_TEST_DSN / IRIS_TEST_REDIS not set; skipping logging e2e test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, dbCleanup, err := data.NewDB(ctx, conf.Database{DSN: dsn, MaxConns: 4, MinConns: 1})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(dbCleanup)
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	streams, streamsCleanup, err := data.NewStreams(ctx, conf.Redis{Addr: redisAddr, ConsumerName: "e2e-logging"})
	if err != nil {
		t.Fatalf("connect redis: %v", err)
	}
	t.Cleanup(streamsCleanup)

	repo := data.NewMailOpsRepo(db)
	const stream = "iris.mail.events.e2e"

	// Start kumod with the routing snapshot (its log hook targets `stream`).
	r := startRig(t, routingSnapshot(kumodIP))

	// Consume the log stream the generated policy feeds. Starting the worker
	// before injecting ensures the consumer group exists for kumod's XADDs.
	w := worker.NewLogStreamWorker(streams, repo, nil, nil, stream, biz.NewLogger("error"))
	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	go func() { _ = w.Run(workerCtx) }()
	time.Sleep(time.Second)

	recipient := "log-probe@sink.test"
	r.inject(recipient, "X-Mail-Class: bulk")

	// Poll mail_records for this recipient until both a Reception and a Delivery
	// have been ingested (or we time out).
	deadline := time.Now().Add(40 * time.Second)
	var statuses map[string]bool
	for time.Now().Before(deadline) {
		recs, err := repo.ListMailRecords(ctx, biz.MailFilter{}, biz.NormalizePage(0, ""))
		if err != nil {
			t.Fatalf("list mail records: %v", err)
		}
		statuses = map[string]bool{}
		for _, rec := range recs {
			if rec.Recipient == recipient {
				statuses[rec.Status] = true
			}
		}
		if statuses[biz.MailReceived] && statuses[biz.MailSent] {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !statuses[biz.MailReceived] {
		t.Errorf("expected a Reception (status %q) for %s in mail_records; got %v\nkumod logs:\n%s",
			biz.MailReceived, recipient, statuses, lastLines(r.kumodLogs(), 15))
	}
	if !statuses[biz.MailSent] {
		t.Errorf("expected a Delivery (status %q) for %s in mail_records; got %v",
			biz.MailSent, recipient, statuses)
	}
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
