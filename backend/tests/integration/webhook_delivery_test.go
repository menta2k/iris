package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/worker"
)

// TestWebhookFiresOnReception verifies the full inbound-webhook pipeline end to
// end: a Reception log record on the mail stream is ingested by the
// LogStreamWorker, which (via the webhook producer) enqueues a delivery event
// that the WebhookWorker fans out to the matching destination — recording a
// delivery. This exercises the producer that was previously missing, so
// webhooks actually fire.
func TestWebhookFiresOnReception(t *testing.T) {
	db := setupDB(t)
	streams := setupStreams(t)

	// Capture webhook deliveries at an in-process destination.
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		bodies = append(bodies, m)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	inboundRepo := data.NewInboundRepo(db)
	inboundUC := biz.NewInboundUsecase(inboundRepo, nil, true) // allow http destinations
	if _, err := inboundUC.CreateWebhookRule(ownerCtx(), &biz.WebhookRule{
		Name: "hook", MatchType: biz.MatchRecipientDomain, MatchValue: "inbound.example",
		DestinationURL: srv.URL, TimeoutSeconds: 5,
		RetryPolicy: biz.RetryPolicy{MaxAttempts: 1, BackoffSeconds: 1},
	}); err != nil {
		t.Fatalf("create webhook rule: %v", err)
	}

	mailRepo := data.NewMailOpsRepo(db)
	const stream = "iris.mail.events.webhooktest"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := streams.EnsureGroup(ctx, stream, "iris-logstream"); err != nil {
		t.Fatalf("ensure group: %v", err)
	}

	// LogStreamWorker (with the webhook producer) + WebhookWorker.
	logWorker := worker.NewLogStreamWorker(streams, mailRepo, nil, nil, stream, biz.NewLogger("error")).
		WithWebhooks(worker.NewWebhookProducer(streams))
	hookWorker := worker.NewWebhookWorker(streams, inboundUC, biz.NewLogger("error"))
	go func() { _ = logWorker.Run(ctx) }()
	go func() { _ = hookWorker.Run(ctx) }()

	// A received message for the hooked domain.
	recipient := "user@inbound.example"
	record := `{"type":"Reception","id":"wh-1","sender":"s@ext.example","recipient":"` + recipient + `","meta":{}}`
	if _, err := streams.Publish(ctx, stream, map[string]any{"type": "Reception", "data": record}); err != nil {
		t.Fatalf("publish reception: %v", err)
	}

	// The webhook should fire and a delivery event be recorded.
	deadline := time.Now().Add(12 * time.Second)
	var delivered bool
	for time.Now().Before(deadline) {
		mu.Lock()
		got := len(bodies)
		mu.Unlock()
		if got > 0 {
			delivered = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	cancel()

	if !delivered {
		t.Fatal("expected the webhook destination to receive a POST for the received message")
	}
	mu.Lock()
	payload := bodies[0]
	mu.Unlock()
	if payload["recipient"] != recipient || payload["event"] != "reception" {
		t.Fatalf("unexpected webhook payload: %+v", payload)
	}

	// And the delivery is listable, joined with the webhook name + recipient.
	deliveries, err := inboundRepo.ListWebhookDeliveries(context.Background(), biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	var found *biz.WebhookDeliveryEvent
	for _, e := range deliveries {
		if e.Status == biz.WebhookDelivered && e.WebhookName == "hook" {
			found = e
		}
	}
	if found == nil {
		t.Fatalf("expected a delivered webhook delivery for rule 'hook', got %d events", len(deliveries))
	}
	if found.Recipient != recipient {
		t.Errorf("delivery recipient = %q, want %q (joined from mail_records)", found.Recipient, recipient)
	}
}
