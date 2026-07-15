// Package metrics defines the Prometheus metrics Iris exposes at /metrics. They
// are driven primarily by the log-stream worker (which ingests KumoMTA's
// structured log records) and the webhook worker. Counters are registered with
// the default Prometheus registry, which the /metrics handler scrapes.
//
// Cardinality note: recipient_domain and vmta are open-ended labels. For a
// deployment with a bounded set of destination domains and VMTAs this is fine;
// at very large scale consider capping to top-N domains.
package metrics

import "github.com/prometheus/client_golang/prometheus"

const labelUnknown = "unknown"

var (
	// MailEvents counts mail events ingested from the log stream, sliced by
	// delivery status, mail class, and recipient domain. Answers "how much mail
	// to gmail.com", "per mail class + domain", and success(sent)/fail(bounced)
	// breakdowns.
	MailEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "iris_mail_events_total",
		Help: "Mail events ingested from the KumoMTA log stream, by status, mail class, recipient domain, and receiving cluster node.",
	}, []string{"status", "mailclass", "recipient_domain", "node"})

	// VMTAEvents counts outbound mail events by the egress source (VMTA) that
	// handled them and the resulting status. Answers "mails per VMTA".
	VMTAEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "iris_vmta_events_total",
		Help: "Outbound mail events by egress source (VMTA), status, and queueing cluster node.",
	}, []string{"vmta", "status", "node"})

	// Bounces counts bounces by type (hard/soft/dsn) and mail class.
	Bounces = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "iris_bounces_total",
		Help: "Bounces by type (hard, soft, dsn) and mail class.",
	}, []string{"type", "mailclass"})

	// WebhookExecutions counts webhook delivery executions by webhook and result.
	WebhookExecutions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "iris_webhook_executions_total",
		Help: "Webhook delivery executions by webhook and result (delivered, retrying, failed).",
	}, []string{"webhook", "result"})

	// MailQueueTime measures how long a message spent queued — from its creation
	// (Reception) to a successful Delivery — sliced by mail class. Observed on
	// Delivery events only. A histogram (not a gauge/counter) so the dashboard can
	// draw the latency distribution and quantiles (p50/p90/p99). The mailclass
	// label serves both views: aggregate over it (sum by le) for the global
	// distribution, or keep it for the per-mail-class distribution.
	//
	// Buckets span 100ms to 1h: fast deliveries land sub-second, while messages
	// that defer-then-deliver can sit in the queue for minutes.
	MailQueueTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "iris_mail_queue_time_seconds",
		Help:    "Time a message spent queued from Reception to successful Delivery, in seconds, by mail class.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
	}, []string{"mailclass", "node"})

	// Self-monitoring gauges: current host resource usage, refreshed by the
	// monitor worker each sample so Prometheus can scrape and chart them over
	// time (CPU/memory as %, disk per mount path).
	SystemCPUPercent = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "iris_system_cpu_percent",
		Help: "Host CPU usage percent (0-100).",
	})
	SystemMemoryPercent = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "iris_system_memory_percent",
		Help: "Host memory usage percent (0-100).",
	})
	SystemMemoryUsedBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "iris_system_memory_used_bytes",
		Help: "Host memory used in bytes.",
	})
	SystemDiskUsedPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "iris_system_disk_used_percent",
		Help: "Host filesystem usage percent (0-100) by mount path.",
	}, []string{"path"})
	SystemDiskUsedBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "iris_system_disk_used_bytes",
		Help: "Host filesystem used bytes by mount path.",
	}, []string{"path"})
	SystemDiskTotalBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "iris_system_disk_total_bytes",
		Help: "Host filesystem total bytes by mount path.",
	}, []string{"path"})
)

func init() {
	prometheus.MustRegister(MailEvents, VMTAEvents, Bounces, WebhookExecutions, MailQueueTime,
		SystemCPUPercent, SystemMemoryPercent, SystemMemoryUsedBytes,
		SystemDiskUsedPercent, SystemDiskUsedBytes, SystemDiskTotalBytes)
}

// SystemDiskSample is one filesystem's usage passed to RecordSystem.
type SystemDiskSample struct {
	Path        string
	UsedPercent float64
	UsedBytes   float64
	TotalBytes  float64
}

// RecordSystem publishes the latest host snapshot to the system gauges. The disk
// gauge vectors are reset first so a path dropped from monitoring stops emitting.
func RecordSystem(cpuPercent, memPercent, memUsedBytes float64, disks []SystemDiskSample) {
	SystemCPUPercent.Set(cpuPercent)
	SystemMemoryPercent.Set(memPercent)
	SystemMemoryUsedBytes.Set(memUsedBytes)
	SystemDiskUsedPercent.Reset()
	SystemDiskUsedBytes.Reset()
	SystemDiskTotalBytes.Reset()
	for _, d := range disks {
		SystemDiskUsedPercent.WithLabelValues(d.Path).Set(d.UsedPercent)
		SystemDiskUsedBytes.WithLabelValues(d.Path).Set(d.UsedBytes)
		SystemDiskTotalBytes.WithLabelValues(d.Path).Set(d.TotalBytes)
	}
}

// RecordMailEvent records a single mail event (Reception/Delivery/Bounce).
func RecordMailEvent(status, mailclass, recipientDomain, node string) {
	MailEvents.WithLabelValues(or(status), or(mailclass), or(recipientDomain), or(node)).Inc()
}

// RecordVMTAEvent records an outbound mail event attributed to a VMTA (egress
// source). A no-op when the VMTA is unknown (e.g. inbound receptions).
func RecordVMTAEvent(vmta, status, node string) {
	if vmta == "" {
		return
	}
	VMTAEvents.WithLabelValues(vmta, or(status), or(node)).Inc()
}

// RecordBounce records a bounce by type and mail class.
func RecordBounce(bounceType, mailclass string) {
	Bounces.WithLabelValues(or(bounceType), or(mailclass)).Inc()
}

// RecordWebhookExecution records a webhook delivery attempt outcome.
func RecordWebhookExecution(webhook, result string) {
	WebhookExecutions.WithLabelValues(or(webhook), or(result)).Inc()
}

// RecordQueueTime observes the queue latency (Reception → Delivery), in seconds,
// of a delivered message for the given mail class. Negative durations (clock
// skew) are dropped.
func RecordQueueTime(mailclass, node string, seconds float64) {
	if seconds < 0 {
		return
	}
	MailQueueTime.WithLabelValues(or(mailclass), or(node)).Observe(seconds)
}

// or substitutes a stable placeholder for an empty label value so series do not
// carry empty labels.
func or(v string) string {
	if v == "" {
		return labelUnknown
	}
	return v
}
