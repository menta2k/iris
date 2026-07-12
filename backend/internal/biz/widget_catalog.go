package biz

import (
	"regexp"
	"slices"
	"strings"
)

// Widget visualization types the frontend can render.
const (
	WidgetVizLine  = "line"
	WidgetVizArea  = "area"
	WidgetVizBar   = "bar"
	WidgetVizGauge = "gauge"
	WidgetVizStat  = "stat"
)

// WidgetDef is a curated dashboard widget: a safe, named metric query with its
// visualization metadata. The PromQL template may contain two placeholders
// resolved server-side: $window (the rate window for the selected range) and
// $groupBy (expands to "by (<label>)" when a group-by label is chosen).
type WidgetDef struct {
	Key             string
	Category        string
	Title           string
	Description     string
	Unit            string // display unit: msg/s, count, bytes, percent, seconds
	Viz             string
	PromQLTemplate  string
	SupportsGroupBy bool
	GroupByLabels   []string
	DefaultRange    string
	// Instant widgets return a single current value (stat/gauge) via an instant
	// query rather than a range query.
	Instant bool
}

// widgetCatalog is the curated set of KumoMTA + iris metrics offered as widgets.
// KumoMTA metrics (kumod /metrics) require Prometheus to scrape kumod; where a
// metric is not scraped the widget renders "no data" (empty series), same as the
// existing mail-flow panel.
var widgetCatalog = []WidgetDef{
	// --- Messages ---
	{Key: "kumo_messages_delivered_rate", Category: "Messages", Title: "Delivered / sec", Description: "Successful deliveries per second.", Unit: "msg/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_messages_delivered[$window])) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"provider", "pool", "source"}, DefaultRange: "6h"},
	{Key: "kumo_messages_received_rate", Category: "Messages", Title: "Received / sec", Description: "Inbound messages received per second.", Unit: "msg/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_messages_received[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_messages_fail_rate", Category: "Messages", Title: "Failed / sec", Description: "Permanent delivery failures (bounces) per second.", Unit: "msg/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_messages_fail[$window])) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"provider", "pool", "source"}, DefaultRange: "6h"},
	{Key: "kumo_messages_transfail_rate", Category: "Messages", Title: "Transient failures / sec", Description: "Transient (retryable) failures per second.", Unit: "msg/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_messages_transfail[$window])) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"provider", "pool", "source"}, DefaultRange: "6h"},
	{Key: "kumo_message_count", Category: "Messages", Title: "Messages in system", Description: "Messages currently held in memory.", Unit: "count", Viz: WidgetVizStat, Instant: true,
		PromQLTemplate: `sum(message_count)`, DefaultRange: "1h"},

	// --- Queues ---
	{Key: "kumo_scheduled_count", Category: "Queues", Title: "Scheduled queue depth", Description: "Messages awaiting their next delivery attempt.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(scheduled_count) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"provider", "pool"}, DefaultRange: "6h"},
	{Key: "kumo_ready_count", Category: "Queues", Title: "Ready queue depth", Description: "Messages ready to send right now.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(ready_count) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"provider", "pool"}, DefaultRange: "6h"},
	{Key: "kumo_queued_by_provider", Category: "Queues", Title: "Queued by provider", Description: "Queued messages grouped by destination provider.", Unit: "count", Viz: WidgetVizBar, Instant: true,
		PromQLTemplate: `sum(queued_count_by_provider) by (provider)`, SupportsGroupBy: false, DefaultRange: "1h"},

	// --- Connections ---
	{Key: "kumo_connection_count", Category: "Connections", Title: "Active connections", Description: "Open outbound SMTP connections.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(connection_count) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"provider", "pool"}, DefaultRange: "6h"},
	{Key: "kumo_connections_denied_rate", Category: "Connections", Title: "Connections denied / sec", Description: "Rejected inbound connections per second.", Unit: "conn/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_connections_denied[$window]))`, DefaultRange: "6h"},

	// --- Latency (histograms) ---
	{Key: "kumo_deliver_latency_p95", Category: "Latency", Title: "Delivery latency p95", Description: "95th-percentile message delivery duration.", Unit: "seconds", Viz: WidgetVizLine,
		PromQLTemplate: `histogram_quantile(0.95, sum(rate(deliver_message_latency_rollup_bucket[$window])) by (le))`, DefaultRange: "6h"},
	{Key: "kumo_queue_insert_latency_p95", Category: "Latency", Title: "Queue insert latency p95", Description: "95th-percentile queue insertion duration.", Unit: "seconds", Viz: WidgetVizLine,
		PromQLTemplate: `histogram_quantile(0.95, sum(rate(queue_insert_latency_bucket[$window])) by (le))`, DefaultRange: "6h"},

	// --- Resources ---
	{Key: "kumo_memory_usage", Category: "Resources", Title: "Memory usage", Description: "kumod process memory consumption.", Unit: "bytes", Viz: WidgetVizLine,
		PromQLTemplate: `sum(memory_usage)`, DefaultRange: "6h"},
	{Key: "kumo_cpu_normalized", Category: "Resources", Title: "kumod CPU", Description: "kumod process CPU usage (0-1 normalized).", Unit: "ratio", Viz: WidgetVizLine,
		PromQLTemplate: `process_cpu_usage_normalized`, DefaultRange: "6h"},
	{Key: "kumo_thread_pool_parked", Category: "Resources", Title: "Parked threads", Description: "Idle threads per pool.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(thread_pool_parked) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"pool"}, DefaultRange: "6h"},

	// --- Disk ---
	{Key: "kumo_disk_free_percent", Category: "Disk", Title: "Disk free %", Description: "Minimum free disk space across spool volumes.", Unit: "percent", Viz: WidgetVizGauge, Instant: true,
		PromQLTemplate: `min(disk_free_percent)`, DefaultRange: "1h"},

	// --- DNS / DKIM / DANE ---
	{Key: "kumo_dns_mx_resolve_rate", Category: "DNS/DKIM/DANE", Title: "MX resolutions / sec", Description: "Successful MX lookups per second.", Unit: "ops/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(dns_mx_resolve_status_ok[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_dane_result_rate", Category: "DNS/DKIM/DANE", Title: "DANE results / sec", Description: "DANE validation outcomes per second.", Unit: "ops/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(dane_result_count[$window])) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"result"}, DefaultRange: "6h"},

	// --- Iris (own exported metrics, already scraped) ---
	{Key: "iris_cpu_percent", Category: "Iris", Title: "Host CPU %", Description: "iris host CPU utilization.", Unit: "percent", Viz: WidgetVizGauge, Instant: true,
		PromQLTemplate: `iris_system_cpu_percent`, DefaultRange: "1h"},
	{Key: "iris_memory_percent", Category: "Iris", Title: "Host memory %", Description: "iris host memory utilization.", Unit: "percent", Viz: WidgetVizGauge, Instant: true,
		PromQLTemplate: `iris_system_memory_percent`, DefaultRange: "1h"},
	{Key: "iris_deliveries_rate", Category: "Iris", Title: "Deliveries / min", Description: "iris mail-flow deliveries per minute.", Unit: "msg/min", Viz: WidgetVizArea,
		PromQLTemplate: `sum(rate(iris_mail_events_total{status="` + MailSent + `"}[$window])) * 60`, DefaultRange: "6h"},
}

// WidgetCatalog returns a copy of the curated widget catalog for the API.
func WidgetCatalog() []WidgetDef {
	out := make([]WidgetDef, len(widgetCatalog))
	copy(out, widgetCatalog)
	return out
}

// lookupWidget finds a catalog widget by key.
func lookupWidget(key string) (WidgetDef, bool) {
	for _, w := range widgetCatalog {
		if w.Key == key {
			return w, true
		}
	}
	return WidgetDef{}, false
}

var promLabelRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// resolveTemplate substitutes $window and $groupBy in a catalog PromQL template.
// A group-by is only applied when the def supports it AND the label is in the
// def's allow-list (sanitized against injection).
func (d WidgetDef) resolveTemplate(window, groupBy string) string {
	by := ""
	if d.SupportsGroupBy && groupBy != "" {
		if slices.Contains(d.GroupByLabels, groupBy) && promLabelRe.MatchString(groupBy) {
			by = "by (" + groupBy + ")"
		}
	}
	q := strings.ReplaceAll(d.PromQLTemplate, "$window", window)
	q = strings.ReplaceAll(q, "$groupBy", by)
	return strings.TrimSpace(q)
}
