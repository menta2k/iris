// Package metrics defines the Prometheus metrics surface for iris and
// owns the dedicated *prometheus.Registry that backs the /metrics
// endpoint. Centralising the metric definitions here avoids drift
// between hot-path counters and the documentation operators rely on
// when wiring scrape configs and Grafana dashboards.
//
// Naming convention: `iris_<subsystem>_<unit>` per Prometheus best
// practice. Counters end in `_total`; durations in `_seconds`; sizes
// in `_bytes`.
//
// Cardinality discipline: labels are kept finite. event_type is one of
// kumomta's enum values (~7 strings). mail_class is operator-defined,
// expected ≤ 50 in practice; an Iris instance with thousands of classes
// would have other problems before Prometheus starts complaining.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Build identifies the binary at scrape time. Set Version (and
// optionally GoVersion) at startup so a Grafana dashboard can show
// "you're scraping vXYZ" without parsing logs.
type Build struct {
	Version   string
	GoVersion string
}

// Metrics groups every Iris-defined collector. Pass it down to the
// logstream consumer, suppression resync, etc., so each call site only
// touches its own metric and we keep one place to add a new one.
type Metrics struct {
	reg *prometheus.Registry

	// Log-stream — the most useful operational hook. Every kumomta
	// delivery event flows through here.
	LogEventsTotal      *prometheus.CounterVec   // labels: event_type, mail_class
	LogEventsDropped    *prometheus.CounterVec   // labels: reason (parse_error|persist_error|deadletter)
	LogEventDuration    *prometheus.HistogramVec // labels: event_type
	LogStreamPending    prometheus.Gauge         // XPENDING from kumo.events consumer group

	// Suppression index — gauge tracking how many entries we serve.
	SuppressionEntries *prometheus.GaugeVec // labels: scope (addr|domain)
	SuppressionOpsTotal *prometheus.CounterVec // labels: op (add|remove|resync), result (ok|error)

	// Policy renderer — apply outcomes (the operator-facing button).
	PolicyApplyTotal *prometheus.CounterVec // labels: result (ok|error)

	// kumomta admin client — the upstream we proxy queue/suspend etc.
	// to. Useful for spotting "kumomta unreachable" before users do.
	KumomtaRequestDuration *prometheus.HistogramVec // labels: endpoint, method, result
}

// New creates a fresh Metrics + Registry.
//
// We deliberately use a *private* Registry rather than the global
// prometheus.DefaultRegisterer. That keeps Iris's exposition surface
// scoped to metrics we explicitly defined here — no surprise metrics
// leaking from third-party packages that happen to register against
// the global default. The Go runtime + process collectors are added
// explicitly below because they're useful and cheap.
func New(b Build) *Metrics {
	reg := prometheus.NewRegistry()

	// Standard runtime telemetry.
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// One-shot info gauge so dashboards can pin the version.
	if b.Version != "" {
		info := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "iris_build_info",
			Help: "1 if this iris instance is running; labels carry build metadata.",
		}, []string{"version", "go_version"})
		info.WithLabelValues(b.Version, b.GoVersion).Set(1)
		reg.MustRegister(info)
	}

	m := &Metrics{
		reg: reg,

		LogEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "iris_log_events_total",
			Help: "Count of log records persisted, by kumomta event type and mail class.",
		}, []string{"event_type", "mail_class"}),

		LogEventsDropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "iris_log_events_dropped_total",
			Help: "Count of log records that failed to persist, grouped by reason.",
		}, []string{"reason"}),

		LogEventDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "iris_log_event_processing_duration_seconds",
			Help: "Wall-clock time to parse + persist a single log_event record.",
			// Buckets tuned for the observed range (PG insert ~1-5ms;
			// network blip pushes us into 100ms; 1s is "something's
			// wrong"). Wider than default to catch tail latency.
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"event_type"}),

		LogStreamPending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "iris_log_stream_pending",
			Help: "Number of log entries pending in the kumo.events consumer group (XPENDING).",
		}),

		SuppressionEntries: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "iris_suppression_entries",
			Help: "Number of entries in the Redis suppression index, by scope.",
		}, []string{"scope"}),

		SuppressionOpsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "iris_suppression_ops_total",
			Help: "Suppression-index operations (add/remove/resync), by result.",
		}, []string{"op", "result"}),

		PolicyApplyTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "iris_policy_apply_total",
			Help: "Number of /v1/policy/apply requests, by outcome.",
		}, []string{"result"}),

		KumomtaRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "iris_kumomta_request_duration_seconds",
			Help:    "Latency of HTTP requests to the kumomta admin API.",
			Buckets: prometheus.DefBuckets,
		}, []string{"endpoint", "method", "result"}),
	}

	reg.MustRegister(
		m.LogEventsTotal,
		m.LogEventsDropped,
		m.LogEventDuration,
		m.LogStreamPending,
		m.SuppressionEntries,
		m.SuppressionOpsTotal,
		m.PolicyApplyTotal,
		m.KumomtaRequestDuration,
	)

	return m
}

// Registry returns the underlying *prometheus.Registry — the metrics
// HTTP handler uses this to expose the gathered values via promhttp.
func (m *Metrics) Registry() *prometheus.Registry { return m.reg }

// NewNoop returns a Metrics object with all collectors registered
// against a throwaway registry. Useful in tests and in code paths
// that want to call `m.LogEventsTotal.WithLabelValues(...).Inc()`
// without caring whether the binary is actually exposing metrics.
func NewNoop() *Metrics { return New(Build{}) }
