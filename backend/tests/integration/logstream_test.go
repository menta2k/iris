package integration

import (
	"context"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/worker"
)

// TestLogStreamIngestion publishes KumoMTA-style log records onto the Redis
// stream and verifies the log-stream worker persists them into mail_records,
// and a Bounce additionally into bounce_records — the real path that populates
// the Logs UI.
func TestLogStreamIngestion(t *testing.T) {
	db := setupDB(t)
	streams := setupStreams(t)
	repo := data.NewMailOpsRepo(db)

	const stream = "iris.mail.events.test"
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	mid := "msg-logstream-1"
	records := []string{
		`{"type":"Reception","id":"` + mid + `","sender":"a@send.example","recipient":"b@dest.example","meta":{"tenant":"bulk"}}`,
		`{"type":"Delivery","id":"` + mid + `","sender":"a@send.example","recipient":"b@dest.example","response":{"code":250,"content":"OK"}}`,
		`{"type":"Bounce","id":"` + mid + `","recipient":"c@dest.example","response":{"code":550,"content":"5.1.1 user unknown"}}`,
	}
	if err := streams.EnsureGroup(ctx, stream, "iris-logstream"); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	for _, r := range records {
		if _, err := streams.Publish(ctx, stream, map[string]any{"type": "x", "data": r}); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	w := worker.NewLogStreamWorker(streams, repo, nil, nil, stream, biz.NewLogger("error"))
	done := make(chan struct{})
	go func() { _ = w.Run(ctx); close(done) }()

	// Poll until the three mail events land (or the context times out).
	deadline := time.Now().Add(8 * time.Second)
	var events []*biz.MailRecord
	for time.Now().Before(deadline) {
		evs, err := repo.ListMailRecords(ctx, biz.MailFilter{}, biz.NormalizePage(0, ""))
		if err != nil {
			t.Fatalf("list mail records: %v", err)
		}
		events = filterByMessageID(evs, mid)
		if len(events) >= 3 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	cancel()
	<-done

	if len(events) != 3 {
		t.Fatalf("expected 3 mail events for %s, got %d", mid, len(events))
	}
	statuses := map[string]bool{}
	for _, e := range events {
		statuses[e.Status] = true
		if e.RecipientDomain != "dest.example" {
			t.Fatalf("recipient domain not derived: %+v", e)
		}
	}
	for _, want := range []string{biz.MailReceived, biz.MailSent, biz.MailBounced} {
		if !statuses[want] {
			t.Fatalf("missing mail event status %q (have %v)", want, statuses)
		}
	}

	bounces, err := repo.ListBounces(context.Background(), biz.BounceFilter{}, biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list bounces: %v", err)
	}
	found := false
	for _, b := range bounces {
		if b.Recipient == "c@dest.example" && b.SMTPStatus == "550" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a bounce record for the Bounce log event, got %d bounces", len(bounces))
	}
}

func filterByMessageID(in []*biz.MailRecord, mid string) []*biz.MailRecord {
	var out []*biz.MailRecord
	for _, e := range in {
		if e.MessageID == mid {
			out = append(out, e)
		}
	}
	return out
}
