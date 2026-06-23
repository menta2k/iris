//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/worker"
)

// requireTLSSnapshot builds a minimal outbound config (one listener + one VMTA)
// that delivers to sink.test. When requireTLS is set, a TLS policy forces TLS on
// delivery to sink.test — and the rig's sink speaks plain SMTP with no STARTTLS,
// so kumod must refuse to deliver in cleartext.
func requireTLSSnapshot(staticIP string, requireTLS bool) biz.ConfigSnapshot {
	snap := biz.ConfigSnapshot{
		Listeners: []*biz.Listener{
			{ID: "lst-1", Name: "edge", IPAddress: staticIP, Port: 2525, Hostname: "mx.e2e.test",
				Status: biz.ListenerStatusActive},
		},
		VMTAs: []*biz.VMTA{
			{ID: "v1", Name: "vmta-1", ListenerID: "lst-1", IPAddress: staticIP,
				EHLOName: "egress.e2e.test", Status: biz.VMTAStatusActive},
		},
		EgressEHLODefault: "egress.e2e.test",
		HTTPListen:        "127.0.0.1:8000",
	}
	if requireTLS {
		snap.TLSPolicies = []*biz.TLSPolicy{
			{ID: "t1", Domain: "sink.test", Mode: biz.TLSModeRequired, Status: biz.TLSPolicyActive},
		}
	}
	return snap
}

// TestRequireTLSRejectsCleartextDelivery proves the outbound require-TLS policy
// end-to-end through a real kumod. The rig sink speaks plain SMTP (no STARTTLS),
// so:
//
//   - Without a policy, kumod delivers to sink.test in the clear (control).
//   - With a require-TLS policy for sink.test, kumod must NOT deliver in the
//     clear: the sink stays empty and the failed delivery is logged as a
//     non-sent mail event (the "proper log entry about the rejection").
//
// Note on disposition: KumoMTA treats "TLS required but the peer offered no
// STARTTLS" as a delivery failure; whether it surfaces as a TransientFailure
// (deferred) or a Bounce depends on kumod, so the assertion accepts any
// non-sent terminal/intermediate failure status and asserts a 'sent' event is
// never produced.
func TestRequireTLSRejectsCleartextDelivery(t *testing.T) {
	requireE2E(t)
	requireDocker(t)

	// Control: no policy -> kumod delivers to the plain (no-STARTTLS) sink. This
	// proves the sink accepts cleartext, so the rejection below is caused by the
	// policy and not by a broken sink.
	t.Run("delivers_without_policy", func(t *testing.T) {
		r := startRig(t, requireTLSSnapshot(kumodIP, false))
		r.inject("clear@sink.test")
		r.waitForSink(1, 30*time.Second) // fatals on timeout
	})

	// Enforced: require TLS for sink.test.
	t.Run("rejects_and_logs_with_policy", func(t *testing.T) {
		dsn := os.Getenv("IRIS_TEST_DSN")
		redisAddr := os.Getenv("IRIS_TEST_REDIS")
		const stream = "iris.mail.events.e2e"
		recipient := "secure@sink.test"

		snap := requireTLSSnapshot(kumodIP, true)

		// When the dev DB/Redis are available, wire the log pipeline so we can
		// assert the rejection is recorded in mail_records.
		var repo *data.MailOpsRepo
		var ctx context.Context
		if dsn != "" && redisAddr != "" {
			snap.LogStreamRedisURL = "redis://iris-redis:6379"
			snap.LogStreamName = stream

			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
			t.Cleanup(cancel)

			db, dbCleanup, err := data.NewDB(ctx, conf.Database{DSN: dsn, MaxConns: 4, MinConns: 1})
			if err != nil {
				t.Fatalf("connect db: %v", err)
			}
			t.Cleanup(dbCleanup)
			if err := db.Migrate(ctx); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			streams, streamsCleanup, err := data.NewStreams(ctx, conf.Redis{Addr: redisAddr, ConsumerName: "e2e-require-tls"})
			if err != nil {
				t.Fatalf("connect redis: %v", err)
			}
			t.Cleanup(streamsCleanup)

			repo = data.NewMailOpsRepo(db)
			w := worker.NewLogStreamWorker(streams, repo, nil, nil, stream, biz.NewLogger("error"))
			workerCtx, stopWorker := context.WithCancel(ctx)
			t.Cleanup(stopWorker)
			go func() { _ = w.Run(workerCtx) }()
			time.Sleep(time.Second) // let the consumer group form before kumod XADDs
		}

		r := startRig(t, snap)
		r.inject(recipient)

		// Hard proof: no cleartext delivery ever reaches the sink. Give kumod
		// ample time to attempt and fail the delivery; because the sink never
		// offers STARTTLS, a require-TLS delivery can never succeed.
		time.Sleep(15 * time.Second)
		if msgs := r.sinkMessages(); len(msgs) != 0 {
			t.Fatalf("require-TLS domain must not receive cleartext mail; sink captured %d message(s).\nkumod logs:\n%s",
				len(msgs), r.kumodLogs())
		}

		if repo == nil {
			t.Log("IRIS_TEST_DSN / IRIS_TEST_REDIS unset; skipping the logged-rejection assertion (sink-empty proof still ran)")
			return
		}

		// Proper log entry: the failed delivery is recorded as a non-sent event
		// (deferred/bounced/failed) and a 'sent' event is never produced.
		failure := map[string]bool{biz.MailDeferred: true, biz.MailBounced: true, biz.MailFailed: true}
		deadline := time.Now().Add(40 * time.Second)
		var seen map[string]bool
		for time.Now().Before(deadline) {
			recs, err := repo.ListMailRecords(ctx, biz.MailFilter{}, biz.NormalizePage(0, ""))
			if err != nil {
				t.Fatalf("list mail records: %v", err)
			}
			seen = map[string]bool{}
			for _, rec := range recs {
				if rec.Recipient == recipient {
					seen[rec.Status] = true
				}
			}
			hasFailure := false
			for s := range seen {
				if failure[s] {
					hasFailure = true
				}
			}
			if hasFailure {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		if seen[biz.MailSent] {
			t.Fatalf("require-TLS delivery must never be recorded as sent; statuses=%v", seen)
		}
		failed := false
		for s := range seen {
			if failure[s] {
				failed = true
			}
		}
		if !failed {
			t.Fatalf("expected a logged rejection (deferred/bounced/failed) for %s; saw statuses=%v.\nkumod logs:\n%s",
				recipient, seen, r.kumodLogs())
		}
	})
}
