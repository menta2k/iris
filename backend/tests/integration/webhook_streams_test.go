package integration

import (
	"context"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/data"
)

// TestWebhookDeliveryStream verifies inbound mail events can be published to the
// webhook delivery stream and consumed by a consumer group, which underpins the
// webhook worker's retry/dead-letter handling.
func TestWebhookDeliveryStream(t *testing.T) {
	streams := setupStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const group = "test-webhook-group"
	if err := streams.EnsureGroup(ctx, data.StreamWebhookDelivery, group); err != nil {
		t.Fatalf("ensure group: %v", err)
	}

	marker := "user@example.com"
	if _, err := streams.Publish(ctx, data.StreamWebhookDelivery, map[string]any{
		"recipient":      marker,
		"mail_record_id": "11111111-1111-1111-1111-111111111111",
		"payload":        `{"event":"delivered"}`,
	}); err != nil {
		t.Fatalf("publish delivery: %v", err)
	}

	// The group may also read historical messages; find the one we published.
	found := false
	for i := 0; i < 5 && !found; i++ {
		msgs, err := streams.Consume(ctx, data.StreamWebhookDelivery, group, 50, time.Second)
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		for _, m := range msgs {
			if v, _ := m.Values["recipient"].(string); v == marker {
				found = true
			}
			_ = streams.Ack(ctx, data.StreamWebhookDelivery, group, m.ID)
		}
		if len(msgs) == 0 {
			break
		}
	}
	if !found {
		t.Fatal("expected to consume the published webhook delivery message")
	}
}
