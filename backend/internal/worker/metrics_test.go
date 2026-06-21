package worker

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/metrics"
)

// TestLogStreamDrivesMetrics verifies the log-stream worker updates the
// Prometheus counters: a Delivery increments the mail-events (by status/class/
// domain) and per-VMTA series, and a hard Bounce increments the bounce series.
func TestLogStreamDrivesMetrics(t *testing.T) {
	store := newFakeBounceStore()
	w := newWorker(store, store, nil)
	ctx := context.Background()

	mailBefore := testutil.ToFloat64(metrics.MailEvents.WithLabelValues("sent", "bulk", "gmail.com"))
	vmtaBefore := testutil.ToFloat64(metrics.VMTAEvents.WithLabelValues("vmta-a", "sent"))
	bounceBefore := testutil.ToFloat64(metrics.Bounces.WithLabelValues("hard", "promo"))

	// A successful delivery to gmail.com via vmta-a, class "bulk".
	w.handle(ctx, data.StreamMessage{Values: map[string]any{"data": `{"type":"Delivery","id":"m1",` +
		`"recipient":"user@gmail.com","egress_source":"vmta-a","meta":{"mailclass":"bulk"}}`}})

	// A hard bounce, class "promo".
	w.handle(ctx, data.StreamMessage{Values: map[string]any{"data": `{"type":"Bounce","id":"m2",` +
		`"recipient":"x@dest.example","meta":{"mailclass":"promo"},"response":{"code":550,"content":"5.1.1"}}`}})

	if got := testutil.ToFloat64(metrics.MailEvents.WithLabelValues("sent", "bulk", "gmail.com")) - mailBefore; got != 1 {
		t.Errorf("mail_events{sent,bulk,gmail.com} delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.VMTAEvents.WithLabelValues("vmta-a", "sent")) - vmtaBefore; got != 1 {
		t.Errorf("vmta_events{vmta-a,sent} delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.Bounces.WithLabelValues("hard", "promo")) - bounceBefore; got != 1 {
		t.Errorf("bounces{hard,promo} delta = %v, want 1", got)
	}
}
