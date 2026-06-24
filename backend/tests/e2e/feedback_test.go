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

// arfReport is a minimal RFC 5965 ARF (abuse feedback) report. kumod parses it
// when log_arf is enabled for the destination domain and emits a Feedback log
// record carrying these fields.
func arfReport(complainant string) string {
	lines := []string{
		"--b",
		`Content-Type: text/plain; charset="US-ASCII"`,
		"",
		"This is an abuse report for a message received from the listed sender.",
		"",
		"--b",
		"Content-Type: message/feedback-report",
		"",
		"Feedback-Type: abuse",
		"User-Agent: ExampleFBL/1.0",
		"Version: 1",
		"Original-Mail-From: <bounce@sender.test>",
		"Original-Rcpt-To: " + complainant,
		"Reporting-MTA: dns; mx.provider.test",
		"",
		"--b",
		"Content-Type: message/rfc822",
		"",
		"From: newsletter@sender.test",
		"To: " + complainant,
		"Subject: the original campaign",
		"",
		"original message body",
		"--b--",
		"",
	}
	return strings.Join(lines, "\r\n")
}

// TestFeedbackReportAutoSuppresses proves the FBL pipeline through a real kumod:
// an ARF report sent to the configured FBL domain is parsed by kumod (log_arf),
// emitted as a Feedback log record, streamed via the log hook, and ingested by
// the LogStreamWorker — which persists a feedback report and auto-suppresses the
// complainant.
//
// Needs the dev DB/Redis (IRIS_TEST_DSN / IRIS_TEST_REDIS).
func TestFeedbackReportAutoSuppresses(t *testing.T) {
	requireE2E(t)
	requireDocker(t)
	dsn := os.Getenv("IRIS_TEST_DSN")
	redisAddr := os.Getenv("IRIS_TEST_REDIS")
	if dsn == "" || redisAddr == "" {
		t.Skip("IRIS_TEST_DSN / IRIS_TEST_REDIS not set; skipping feedback e2e test")
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
	streams, streamsCleanup, err := data.NewStreams(ctx, conf.Redis{Addr: redisAddr, ConsumerName: "e2e-fbl"})
	if err != nil {
		t.Fatalf("connect redis: %v", err)
	}
	t.Cleanup(streamsCleanup)

	mailRepo := data.NewMailOpsRepo(db)
	safetyRepo := data.NewDomainSafetyRepo(db)
	const stream = "iris.mail.events.e2e"

	// Enable ARF parsing at the FBL domain via an approved feedback-loop endpoint.
	snap := routingSnapshot(kumodIP)
	snap.FBLEndpoints = []*biz.FBLEndpoint{
		{Domain: "fbl.test", FeedbackAddress: "fbl@fbl.test", Status: biz.FBLApproved},
	}
	r := startRig(t, snap)

	w := worker.NewLogStreamWorker(streams, mailRepo, safetyRepo, nil, stream, biz.NewLogger("error"))
	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	go func() { _ = w.Run(workerCtx) }()
	time.Sleep(time.Second)

	complainant := "angry-user@subscriber.test"
	r.injectFull("complaints@provider.test", "fbl@fbl.test", arfReport(complainant),
		"MIME-Version: 1.0",
		`Content-Type: multipart/report; report-type=feedback-report; boundary="b"`)

	deadline := time.Now().Add(40 * time.Second)
	var gotReport, gotSuppress bool
	for time.Now().Before(deadline) {
		reports, err := mailRepo.ListFeedbackReports(ctx, biz.NormalizePage(0, ""))
		if err != nil {
			t.Fatalf("list feedback reports: %v", err)
		}
		for _, f := range reports {
			if f.Recipient == complainant {
				gotReport = true
			}
		}
		if ok, err := safetyRepo.IsSuppressed(ctx, complainant); err == nil && ok {
			gotSuppress = true
		}
		if gotReport && gotSuppress {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !gotReport {
		t.Errorf("expected a feedback_report for complainant %s\nkumod logs:\n%s",
			complainant, lastLines(r.kumodLogs(), 20))
	}
	if !gotSuppress {
		t.Errorf("expected complainant %s to be auto-suppressed via the FBL pipeline", complainant)
	}
}

// TestFeedbackAwaitingForwards proves the awaiting-approval path through a real
// kumod: an endpoint that is awaiting approval relays its feedback domain and
// forwards mail arriving at its feedback address to the forward address (by
// rewriting the recipient) so it delivers outbound — instead of being ARF-parsed
// and dropped. The sink stands in for the human approval mailbox.
func TestFeedbackAwaitingForwards(t *testing.T) {
	requireE2E(t)
	requireDocker(t)

	// Forward to the sink's domain so kumod relays the rewritten recipient there.
	snap := routingSnapshot(kumodIP)
	snap.FBLEndpoints = []*biz.FBLEndpoint{
		{Domain: "fbl.test", FeedbackAddress: "fbl@fbl.test", ForwardAddress: "approver@sink.test", Status: biz.FBLAwaitingApproval},
	}
	r := startRig(t, snap)

	// A mailbox provider's enrollment-confirmation mail arrives at the feedback
	// address while the loop is awaiting approval.
	r.injectFull("noreply@provider.test", "fbl@fbl.test",
		"Your feedback loop confirmation code is 123456.",
		"Subject: Confirm your feedback loop")

	msgs := r.waitForSink(1, 40*time.Second)
	var forwarded bool
	for _, m := range msgs {
		for _, rcpt := range m.Rcpts {
			if rcpt == "approver@sink.test" {
				forwarded = true
			}
		}
	}
	if !forwarded {
		t.Errorf("expected feedback mail forwarded to approver@sink.test; sink got %+v\nkumod logs:\n%s",
			msgs, lastLines(r.kumodLogs(), 20))
	}
}
