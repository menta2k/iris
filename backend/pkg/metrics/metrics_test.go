package metrics

import (
	"bytes"
	"strings"
	"testing"

	"github.com/prometheus/common/expfmt"
)

// Smoke-test that the exposition surface looks the way operators
// expect. Catches typos in metric names without requiring a running
// scrape loop.
func TestExposedNamesAndLabels(t *testing.T) {
	m := New(Build{Version: "test-1.0", GoVersion: "go1.99"})

	// Drive a representative sample so each metric appears in the
	// gathered output. WithLabelValues on a CounterVec/HistogramVec is
	// what materialises the time series.
	m.LogEventsTotal.WithLabelValues("Reception", "tx").Inc()
	m.LogEventsTotal.WithLabelValues("Delivery", "").Inc()
	m.LogEventsDropped.WithLabelValues("parse_error").Inc()
	m.LogEventDuration.WithLabelValues("Reception").Observe(0.002)
	m.LogStreamPending.Set(7)
	m.SuppressionEntries.WithLabelValues("addr").Set(123)
	m.SuppressionOpsTotal.WithLabelValues("add", "ok").Inc()
	m.PolicyApplyTotal.WithLabelValues("ok").Inc()
	m.KumomtaRequestDuration.WithLabelValues("/api/admin/queues", "GET", "ok").Observe(0.01)

	mfs, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var buf bytes.Buffer
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		if err := enc.Encode(mf); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	got := buf.Bytes()

	mustContain := []string{
		"iris_build_info",
		`iris_build_info{go_version="go1.99",version="test-1.0"} 1`,
		"iris_log_events_total",
		`iris_log_events_total{event_type="Reception",mail_class="tx"} 1`,
		"iris_log_events_dropped_total",
		"iris_log_event_processing_duration_seconds",
		"iris_log_stream_pending",
		"iris_suppression_entries",
		"iris_suppression_ops_total",
		"iris_policy_apply_total",
		"iris_kumomta_request_duration_seconds",
		// Standard runtime metrics added by the collectors registered
		// in New(); their presence proves the registry is wired up.
		"go_goroutines",
		"process_cpu_seconds_total",
	}
	out := string(got)
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in /metrics output, missing", want)
		}
	}
}
