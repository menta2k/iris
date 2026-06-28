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

// TestVERPEnvelopeRewrite proves the generated VERP policy end-to-end through a
// real kumod: with a bounce domain + VERP secret, kumod rewrites the outbound
// envelope sender (smtp_client_message_sending → set_sender) to
// b+<hmac>.<msgid>@<bounce_domain>. The sink records the MAIL FROM kumod used.
func TestVERPEnvelopeRewrite(t *testing.T) {
	requireE2E(t)
	requireDocker(t)

	const secret = "verp-e2e-secret"
	snap := routingSnapshot(kumodIP)
	snap.BounceDomain = "bounce.e2e.test"
	snap.BounceVerpSecret = secret
	r := startRig(t, snap)

	// Deliver one message to the sink (sink.test MX → sink via the rig resolver).
	r.injectAs("newsletter@sender.test", "user@sink.test")

	msgs := r.waitForSink(1, 40*time.Second)
	if len(msgs) == 0 {
		t.Fatalf("sink received no message\nkumod logs:\n%s", lastLines(r.kumodLogs(), 20))
	}
	from := msgs[0].MailFrom
	msgID, signed, ok := biz.ParseBounceVERP(secret, from)
	if !ok {
		t.Fatalf("MAIL FROM %q is not a VERP return-path", from)
	}
	if !signed {
		t.Fatalf("VERP signature did not verify for %q", from)
	}
	if !strings.HasSuffix(strings.ToLower(from), "@bounce.e2e.test") {
		t.Fatalf("VERP not rooted at the bounce domain: %q", from)
	}
	if msgID == "" {
		t.Fatalf("VERP carried no message id: %q", from)
	}
}

// TestBounceClassificationE2E proves the bounce classifier end-to-end: the sink
// rejects a recipient with a 5xx, kumod (with the IANA classifier loaded)
// classifies the failure and streams a Bounce log record, and the LogStreamWorker
// persists a bounce_record carrying the classification.
func TestBounceClassificationE2E(t *testing.T) {
	requireE2E(t)
	requireDocker(t)
	dsn := os.Getenv("IRIS_TEST_DSN")
	redisAddr := os.Getenv("IRIS_TEST_REDIS")
	if dsn == "" || redisAddr == "" {
		t.Skip("IRIS_TEST_DSN / IRIS_TEST_REDIS not set; skipping classifier e2e test")
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
	streams, streamsCleanup, err := data.NewStreams(ctx, conf.Redis{Addr: redisAddr, ConsumerName: "e2e-classify"})
	if err != nil {
		t.Fatalf("connect redis: %v", err)
	}
	t.Cleanup(streamsCleanup)
	repo := data.NewMailOpsRepo(db)
	const stream = "iris.mail.events.e2e"

	snap := routingSnapshot(kumodIP)
	snap.BounceClassifierFile = "/opt/kumomta/share/bounce_classifier/iana.toml"
	r := startRig(t, snap)

	// The sink rejects this recipient permanently → kumod logs a Bounce.
	recipient := "classify-victim@sink.test"
	r.programBounce(recipient, "rcpt", 550, "5.1.1 <"+recipient+"> No such user here")

	w := worker.NewLogStreamWorker(streams, repo, nil, nil, stream, biz.NewLogger("error"))
	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	go func() { _ = w.Run(workerCtx) }()
	time.Sleep(time.Second)

	r.inject(recipient)

	deadline := time.Now().Add(40 * time.Second)
	var classification string
	for time.Now().Before(deadline) {
		bounces, err := repo.ListBounces(ctx, biz.NormalizePage(0, ""))
		if err != nil {
			t.Fatalf("list bounces: %v", err)
		}
		for _, b := range bounces {
			if b.Recipient == recipient && b.Classification != "" {
				classification = b.Classification
			}
		}
		if classification != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if classification == "" {
		t.Fatalf("expected a classified bounce_record for %s\nkumod logs:\n%s",
			recipient, lastLines(r.kumodLogs(), 25))
	}
	t.Logf("kumod classified the bounce as %q", classification)
}
