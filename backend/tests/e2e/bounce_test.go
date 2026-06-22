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

// TestAsyncHardBounceSuppresses proves the async-bounce pipeline through a real
// kumod: when the destination rejects a recipient with a 5xx, kumod emits a
// Bounce log record, the LogStreamWorker records it in bounce_records, and (per
// the default hard-bounce policy) auto-suppresses the recipient.
//
// Needs the dev DB/Redis (IRIS_TEST_DSN / IRIS_TEST_REDIS).
func TestAsyncHardBounceSuppresses(t *testing.T) {
	requireE2E(t)
	requireDocker(t)
	dsn := os.Getenv("IRIS_TEST_DSN")
	redisAddr := os.Getenv("IRIS_TEST_REDIS")
	if dsn == "" || redisAddr == "" {
		t.Skip("IRIS_TEST_DSN / IRIS_TEST_REDIS not set; skipping bounce e2e test")
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
	streams, streamsCleanup, err := data.NewStreams(ctx, conf.Redis{Addr: redisAddr, ConsumerName: "e2e-bounce"})
	if err != nil {
		t.Fatalf("connect redis: %v", err)
	}
	t.Cleanup(streamsCleanup)

	mailRepo := data.NewMailOpsRepo(db)
	safetyRepo := data.NewDomainSafetyRepo(db)
	const stream = "iris.mail.events.e2e"

	r := startRig(t, routingSnapshot(kumodIP))
	// The destination rejects this recipient permanently.
	recipient := "hardbounce@sink.test"
	r.programBounce(recipient, "rcpt", 550, "5.1.1 user unknown")

	// LogStreamWorker with a real suppressor and the default policy (nil =
	// auto-suppress hard bounces).
	w := worker.NewLogStreamWorker(streams, mailRepo, safetyRepo, nil, stream, biz.NewLogger("error"))
	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	go func() { _ = w.Run(workerCtx) }()
	time.Sleep(time.Second)

	r.inject(recipient, "X-Mail-Class: bulk")

	// Poll for the bounce record AND the auto-suppression.
	deadline := time.Now().Add(40 * time.Second)
	var gotBounce, gotSuppress bool
	for time.Now().Before(deadline) {
		bounces, err := mailRepo.ListBounces(ctx, biz.NormalizePage(0, ""))
		if err != nil {
			t.Fatalf("list bounces: %v", err)
		}
		for _, b := range bounces {
			if b.Recipient == recipient && strings.HasPrefix(b.SMTPStatus, "5") {
				gotBounce = true
			}
		}
		if ok, err := safetyRepo.IsSuppressed(ctx, recipient); err == nil && ok {
			gotSuppress = true
		}
		if gotBounce && gotSuppress {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !gotBounce {
		t.Errorf("expected a 5xx bounce_record for %s\nkumod logs:\n%s", recipient, lastLines(r.kumodLogs(), 15))
	}
	if !gotSuppress {
		t.Errorf("expected %s to be auto-suppressed after the hard bounce", recipient)
	}
}

// TestInboundDSNSuppresses proves the inbound bounce-domain (DSN catcher) path
// through a real kumod: mail addressed to the configured bounce domain is
// relayed into the chain, the reception hook routes it to the DSN_TRACKER queue,
// the make.dsn_xadd custom_lua queue XADDs it onto the DSN stream, and the
// DSNWorker records a bounce and suppresses the recipient.
//
// Needs the dev DB/Redis (IRIS_TEST_DSN / IRIS_TEST_REDIS).
func TestInboundDSNSuppresses(t *testing.T) {
	requireE2E(t)
	requireDocker(t)
	dsn := os.Getenv("IRIS_TEST_DSN")
	redisAddr := os.Getenv("IRIS_TEST_REDIS")
	if dsn == "" || redisAddr == "" {
		t.Skip("IRIS_TEST_DSN / IRIS_TEST_REDIS not set; skipping DSN e2e test")
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
	streams, streamsCleanup, err := data.NewStreams(ctx, conf.Redis{Addr: redisAddr, ConsumerName: "e2e-dsn"})
	if err != nil {
		t.Fatalf("connect redis: %v", err)
	}
	t.Cleanup(streamsCleanup)

	mailRepo := data.NewMailOpsRepo(db)
	safetyRepo := data.NewDomainSafetyRepo(db)

	// Enable the bounce/DSN pipeline by configuring a bounce domain.
	snap := routingSnapshot(kumodIP)
	snap.BounceDomain = "bounce.test"
	r := startRig(t, snap)

	// VERP off for this test: the injected address is at the bounce domain, so
	// the envelope recipient is treated as the recipient and suppressed.
	w := worker.NewDSNWorker(streams, mailRepo, safetyRepo, "", biz.DSNStreamName, biz.NewLogger("error"))
	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	go func() { _ = w.Run(workerCtx) }()
	time.Sleep(time.Second)

	// Mail to the bounce domain is caught by the DSN catcher (no external
	// delivery), XADD'd, and the recipient suppressed.
	recipient := "dsn-victim@bounce.test"
	r.inject(recipient)

	deadline := time.Now().Add(40 * time.Second)
	var gotBounce, gotSuppress bool
	for time.Now().Before(deadline) {
		bounces, err := mailRepo.ListBounces(ctx, biz.NormalizePage(0, ""))
		if err != nil {
			t.Fatalf("list bounces: %v", err)
		}
		for _, b := range bounces {
			if b.Recipient == recipient && b.BounceType == "dsn" {
				gotBounce = true
			}
		}
		if ok, err := safetyRepo.IsSuppressed(ctx, recipient); err == nil && ok {
			gotSuppress = true
		}
		if gotBounce && gotSuppress {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !gotBounce {
		t.Errorf("expected a dsn bounce_record for %s\nkumod logs:\n%s", recipient, lastLines(r.kumodLogs(), 20))
	}
	if !gotSuppress {
		t.Errorf("expected %s to be suppressed via the DSN pipeline", recipient)
	}
}
