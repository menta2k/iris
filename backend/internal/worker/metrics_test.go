package worker

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/metrics"
)

// queueTimeStats reads a mail class's queue-time histogram sample count and sum.
func queueTimeStats(t *testing.T, mailclass, node string) (count uint64, sum float64) {
	t.Helper()
	var m dto.Metric
	if err := metrics.MailQueueTime.WithLabelValues(mailclass, node).(prometheus.Metric).Write(&m); err != nil {
		t.Fatalf("write histogram: %v", err)
	}
	return m.GetHistogram().GetSampleCount(), m.GetHistogram().GetSampleSum()
}

// TestLogStreamDrivesMetrics verifies the log-stream worker updates the
// Prometheus counters: a Delivery increments the mail-events (by status/class/
// domain/node) and per-VMTA series, and a hard Bounce increments the bounce
// series. The 'node' label comes from the record meta stamped by the policy.
func TestLogStreamDrivesMetrics(t *testing.T) {
	store := newFakeBounceStore()
	w := newWorker(store, store, nil)
	ctx := context.Background()

	mailBefore := testutil.ToFloat64(metrics.MailEvents.WithLabelValues("sent", "bulk", "gmail.com", "node1"))
	vmtaBefore := testutil.ToFloat64(metrics.VMTAEvents.WithLabelValues("vmta-a", "sent", "node1"))
	bounceBefore := testutil.ToFloat64(metrics.Bounces.WithLabelValues("hard", "promo"))

	// A successful delivery to gmail.com via vmta-a, class "bulk".
	w.handle(ctx, data.StreamMessage{Values: map[string]any{"data": `{"type":"Delivery","id":"m1",` +
		`"recipient":"user@gmail.com","egress_source":"vmta-a","meta":{"mailclass":"bulk","node":"node1"}}`}})

	// A hard bounce, class "promo".
	w.handle(ctx, data.StreamMessage{Values: map[string]any{"data": `{"type":"Bounce","id":"m2",` +
		`"recipient":"x@dest.example","meta":{"mailclass":"promo"},"response":{"code":550,"content":"5.1.1"}}`}})

	if got := testutil.ToFloat64(metrics.MailEvents.WithLabelValues("sent", "bulk", "gmail.com", "node1")) - mailBefore; got != 1 {
		t.Errorf("mail_events{sent,bulk,gmail.com,node1} delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.VMTAEvents.WithLabelValues("vmta-a", "sent", "node1")) - vmtaBefore; got != 1 {
		t.Errorf("vmta_events{vmta-a,sent,node1} delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.Bounces.WithLabelValues("hard", "promo")) - bounceBefore; got != 1 {
		t.Errorf("bounces{hard,promo} delta = %v, want 1", got)
	}
}

// TestLogStreamRecordsQueueLatency verifies a Delivery with a `created`
// timestamp observes the queue-time histogram (Timestamp - Created), by class,
// and that non-Delivery events don't.
func TestLogStreamRecordsQueueLatency(t *testing.T) {
	store := newFakeBounceStore()
	w := newWorker(store, store, nil)
	ctx := context.Background()

	countBefore, sumBefore := queueTimeStats(t, "bulk", "node1")

	// Received at 10:00:00, delivered at 10:00:05 → 5s in the queue.
	w.handle(ctx, data.StreamMessage{Values: map[string]any{"data": `{"type":"Delivery","id":"m1",` +
		`"created":"2026-06-20T10:00:00Z","timestamp":"2026-06-20T10:00:05Z",` +
		`"recipient":"user@gmail.com","egress_source":"vmta-a","meta":{"mailclass":"bulk","node":"node1"}}`}})

	// A Reception carries no queue latency and must not be observed.
	w.handle(ctx, data.StreamMessage{Values: map[string]any{"data": `{"type":"Reception","id":"m1",` +
		`"created":"2026-06-20T10:00:00Z","timestamp":"2026-06-20T10:00:00Z",` +
		`"recipient":"user@gmail.com","meta":{"mailclass":"bulk"}}`}})

	countAfter, sumAfter := queueTimeStats(t, "bulk", "node1")
	if countAfter-countBefore != 1 {
		t.Errorf("queue-time sample count delta = %d, want 1", countAfter-countBefore)
	}
	if sumAfter-sumBefore != 5 {
		t.Errorf("queue-time sum delta = %v, want 5", sumAfter-sumBefore)
	}
}
