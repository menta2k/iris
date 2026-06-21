package integration

import (
	"context"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/worker"
)

// TestQueueAndServiceCommandStreams verifies that the ops producer publishes
// queue and service-control commands to Redis Streams and that a consumer group
// can read and acknowledge them.
func TestQueueAndServiceCommandStreams(t *testing.T) {
	streams := setupStreams(t)
	producer := worker.NewOpsProducer(streams)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const group = "test-ops-group"
	if err := streams.EnsureGroup(ctx, data.StreamQueueCommands, group); err != nil {
		t.Fatalf("ensure queue group: %v", err)
	}
	if err := streams.EnsureGroup(ctx, data.StreamServiceCommands, group); err != nil {
		t.Fatalf("ensure service group: %v", err)
	}

	if _, err := producer.PublishQueueCommand(ctx, "bulk", "pause", "c1"); err != nil {
		t.Fatalf("publish queue command: %v", err)
	}
	if _, err := producer.PublishServiceCommand(ctx, "req-1", "reload"); err != nil {
		t.Fatalf("publish service command: %v", err)
	}

	// The consumer group may read historical messages too, so search for the
	// specific command this test published rather than asserting on the first.
	if !findMessage(ctx, t, streams, data.StreamQueueCommands, group, "action", "pause") {
		t.Fatal("expected to consume the published pause queue command")
	}
	if !findMessage(ctx, t, streams, data.StreamServiceCommands, group, "request_id", "req-1") {
		t.Fatal("expected to consume the published service command for req-1")
	}
}

// findMessage drains up to a few batches looking for a message whose field
// equals want, acknowledging everything it reads.
func findMessage(ctx context.Context, t *testing.T, streams *data.Streams, stream, group, field, want string) bool {
	t.Helper()
	for i := 0; i < 5; i++ {
		msgs, err := streams.Consume(ctx, stream, group, 50, time.Second)
		if err != nil {
			t.Fatalf("consume %s: %v", stream, err)
		}
		found := false
		for _, m := range msgs {
			if v, _ := m.Values[field].(string); v == want {
				found = true
			}
			_ = streams.Ack(ctx, stream, group, m.ID)
		}
		if found {
			return true
		}
		if len(msgs) == 0 {
			break
		}
	}
	return false
}
