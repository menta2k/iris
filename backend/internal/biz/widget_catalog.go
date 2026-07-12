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
	// The *_by_provider_and_source counters carry both labels, so summing them
	// ungrouped equals the plain total while still supporting a provider/source
	// breakdown when grouped.
	{Key: "kumo_messages_delivered_rate", Category: "Messages", Title: "Delivered / sec", Description: "Successful deliveries per second.", Unit: "msg/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_messages_delivered_by_provider_and_source[$window])) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"provider", "source"}, DefaultRange: "6h"},
	{Key: "kumo_messages_received_rate", Category: "Messages", Title: "Received / sec", Description: "Inbound messages accepted by listeners per second.", Unit: "msg/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_messages_received[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_messages_fail_rate", Category: "Messages", Title: "Failed (bounces) / sec", Description: "Permanent delivery failures per second.", Unit: "msg/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_messages_fail_by_provider_and_source[$window])) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"provider", "source"}, DefaultRange: "6h"},
	{Key: "kumo_messages_transfail_rate", Category: "Messages", Title: "Transient failures / sec", Description: "Retryable (transient) failures per second.", Unit: "msg/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_messages_transfail_by_provider_and_source[$window])) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"provider", "source"}, DefaultRange: "6h"},
	{Key: "kumo_message_count", Category: "Messages", Title: "Messages in system", Description: "Messages currently spooled.", Unit: "count", Viz: WidgetVizStat, Instant: true,
		PromQLTemplate: `sum(message_count)`, DefaultRange: "1h"},

	// --- Queues ---
	{Key: "kumo_scheduled_count", Category: "Queues", Title: "Scheduled queue depth", Description: "Messages awaiting their next delivery attempt.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(scheduled_count)`, DefaultRange: "6h"},
	{Key: "kumo_ready_count", Category: "Queues", Title: "Ready queue depth", Description: "Messages ready to send right now.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(ready_count)`, DefaultRange: "6h"},
	{Key: "kumo_queued_by", Category: "Queues", Title: "Queued by provider / pool", Description: "Queued messages, optionally grouped by provider or egress pool.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(queued_count_by_provider_and_pool) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"provider", "pool"}, DefaultRange: "6h"},
	{Key: "kumo_scheduled_queue_count", Category: "Queues", Title: "Active scheduled queues", Description: "Number of live scheduler queues.", Unit: "count", Viz: WidgetVizStat, Instant: true,
		PromQLTemplate: `sum(scheduled_queue_count)`, DefaultRange: "1h"},
	{Key: "kumo_scheduled_by_domain", Category: "Queues", Title: "Scheduled by domain", Description: "Scheduled messages grouped by destination domain.", Unit: "count", Viz: WidgetVizBar, Instant: true,
		PromQLTemplate: `sum(scheduled_by_domain) by (domain)`, DefaultRange: "1h"},
	{Key: "kumo_scheduled_by_tenant", Category: "Queues", Title: "Scheduled by tenant", Description: "Scheduled messages grouped by tenant.", Unit: "count", Viz: WidgetVizBar, Instant: true,
		PromQLTemplate: `sum(scheduled_by_tenant) by (tenant)`, DefaultRange: "1h"},
	{Key: "kumo_ready_full_rate", Category: "Queues", Title: "Ready-queue-full events / sec", Description: "Rate at which the ready queue hit capacity.", Unit: "events/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(ready_full[$window]))`, DefaultRange: "6h"},

	// --- Connections ---
	{Key: "kumo_connection_count", Category: "Connections", Title: "Active connections", Description: "Open outbound SMTP connections.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(connection_count)`, DefaultRange: "6h"},
	{Key: "kumo_connection_by", Category: "Connections", Title: "Connections by provider / pool", Description: "Open connections, optionally grouped by provider or egress pool.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(connection_count_by_provider_and_pool) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"provider", "pool"}, DefaultRange: "6h"},
	{Key: "kumo_total_connections_rate", Category: "Connections", Title: "New connections / sec", Description: "Outbound connections opened per second.", Unit: "conn/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_connection_count[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_connections_denied_rate", Category: "Connections", Title: "Connections denied / sec", Description: "Inbound connections rejected per second.", Unit: "conn/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_connections_denied[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_bind_failures_rate", Category: "Connections", Title: "Source bind failures / sec", Description: "Egress source binding errors per second.", Unit: "events/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(bind_failures[$window]))`, DefaultRange: "6h"},

	// --- Throttling ---
	{Key: "kumo_throttle_message_rate", Category: "Throttling", Title: "Delayed: message-rate throttle / sec", Description: "Deliveries delayed by message-rate throttles.", Unit: "events/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(delayed_due_to_message_rate_throttle[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_throttle_ready_full_rate", Category: "Throttling", Title: "Delayed: ready queue full / sec", Description: "Deliveries delayed because the ready queue was full.", Unit: "events/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(delayed_due_to_ready_queue_full[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_throttle_insert_ready_rate", Category: "Throttling", Title: "Delayed: throttle insert / sec", Description: "Deliveries delayed inserting into a throttled ready queue.", Unit: "events/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(delayed_due_to_throttle_insert_ready[$window]))`, DefaultRange: "6h"},

	// --- SMTP server ---
	{Key: "kumo_smtp_rejections_rate", Category: "SMTP Server", Title: "SMTP rejections / sec", Description: "Inbound messages rejected, grouped by reason.", Unit: "msg/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(smtp_server_rejections[$window])) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"rejection_reason"}, DefaultRange: "6h"},

	// --- Latency (histograms → p95) ---
	{Key: "kumo_deliver_latency_p95", Category: "Latency", Title: "Delivery latency p95", Description: "95th-percentile end-to-end delivery duration.", Unit: "seconds", Viz: WidgetVizLine,
		PromQLTemplate: `histogram_quantile(0.95, sum(rate(deliver_message_latency_rollup_bucket[$window])) by (le))`, DefaultRange: "6h"},
	{Key: "kumo_queue_insert_latency_p95", Category: "Latency", Title: "Queue insert latency p95", Description: "95th-percentile queue insertion duration.", Unit: "seconds", Viz: WidgetVizLine,
		PromQLTemplate: `histogram_quantile(0.95, sum(rate(queue_insert_latency_bucket[$window])) by (le))`, DefaultRange: "6h"},
	{Key: "kumo_queue_resolve_latency_p95", Category: "Latency", Title: "Queue resolve latency p95", Description: "95th-percentile queue resolution duration.", Unit: "seconds", Viz: WidgetVizLine,
		PromQLTemplate: `histogram_quantile(0.95, sum(rate(queue_resolve_latency_bucket[$window])) by (le))`, DefaultRange: "6h"},
	{Key: "kumo_ready_insert_latency_p95", Category: "Latency", Title: "Ready-queue insert latency p95", Description: "95th-percentile ready-queue insertion duration.", Unit: "seconds", Viz: WidgetVizLine,
		PromQLTemplate: `histogram_quantile(0.95, sum(rate(ready_queue_insert_latency_bucket[$window])) by (le))`, DefaultRange: "6h"},
	{Key: "kumo_smtp_txn_duration_p95", Category: "Latency", Title: "SMTP transaction p95", Description: "95th-percentile inbound SMTP transaction duration.", Unit: "seconds", Viz: WidgetVizLine,
		PromQLTemplate: `histogram_quantile(0.95, sum(rate(smtpsrv_transaction_duration_bucket[$window])) by (le))`, DefaultRange: "6h"},
	{Key: "kumo_smtp_process_data_p95", Category: "Latency", Title: "SMTP data processing p95", Description: "95th-percentile message-processing duration.", Unit: "seconds", Viz: WidgetVizLine,
		PromQLTemplate: `histogram_quantile(0.95, sum(rate(smtpsrv_process_data_duration_bucket[$window])) by (le))`, DefaultRange: "6h"},
	{Key: "kumo_message_save_latency_p95", Category: "Latency", Title: "Spool save latency p95", Description: "95th-percentile message persistence duration.", Unit: "seconds", Viz: WidgetVizLine,
		PromQLTemplate: `histogram_quantile(0.95, sum(rate(message_save_latency_bucket[$window])) by (le))`, DefaultRange: "6h"},

	// --- Resources ---
	{Key: "kumo_memory_usage", Category: "Resources", Title: "Memory usage", Description: "kumod heap allocation.", Unit: "bytes", Viz: WidgetVizLine,
		PromQLTemplate: `sum(memory_usage)`, DefaultRange: "6h"},
	{Key: "kumo_memory_usage_rust", Category: "Resources", Title: "Rust allocator memory", Description: "Rust allocator memory usage.", Unit: "bytes", Viz: WidgetVizLine,
		PromQLTemplate: `sum(memory_usage_rust)`, DefaultRange: "6h"},
	{Key: "kumo_memory_over_limit_rate", Category: "Resources", Title: "Memory over-limit events / sec", Description: "Rate of memory-limit-exceeded events.", Unit: "events/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(memory_over_limit_count[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_cpu_normalized", Category: "Resources", Title: "kumod CPU", Description: "kumod CPU usage relative to core count (0-1).", Unit: "ratio", Viz: WidgetVizLine,
		PromQLTemplate: `process_cpu_usage_normalized`, DefaultRange: "6h"},
	{Key: "kumo_system_cpu_normalized", Category: "Resources", Title: "System CPU", Description: "Host CPU usage relative to core count (0-1).", Unit: "ratio", Viz: WidgetVizLine,
		PromQLTemplate: `system_cpu_usage_normalized`, DefaultRange: "6h"},
	{Key: "kumo_thread_pool_parked", Category: "Resources", Title: "Parked threads", Description: "Idle worker threads.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(thread_pool_parked)`, DefaultRange: "6h"},
	{Key: "kumo_thread_pool_size", Category: "Resources", Title: "Thread pool size", Description: "Total worker threads.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(thread_pool_size)`, DefaultRange: "6h"},

	// --- Disk ---
	{Key: "kumo_disk_free_percent", Category: "Disk", Title: "Disk free %", Description: "Minimum free disk space across spool volumes.", Unit: "percent", Viz: WidgetVizGauge, Instant: true,
		PromQLTemplate: `min(disk_free_percent)`, DefaultRange: "1h"},
	{Key: "kumo_disk_free_bytes", Category: "Disk", Title: "Disk free bytes", Description: "Minimum free disk space in bytes.", Unit: "bytes", Viz: WidgetVizStat, Instant: true,
		PromQLTemplate: `min(disk_free_bytes)`, DefaultRange: "1h"},
	{Key: "kumo_disk_free_inodes_percent", Category: "Disk", Title: "Disk free inodes %", Description: "Minimum free inodes across spool volumes.", Unit: "percent", Viz: WidgetVizGauge, Instant: true,
		PromQLTemplate: `min(disk_free_inodes_percent)`, DefaultRange: "1h"},

	// --- Spool / storage ---
	{Key: "kumo_data_resident_count", Category: "Spool", Title: "Resident message bodies", Description: "Message bodies held in memory.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(message_data_resident_count)`, DefaultRange: "6h"},
	{Key: "kumo_meta_resident_count", Category: "Spool", Title: "Resident metadata", Description: "Message metadata held in memory.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(message_meta_resident_count)`, DefaultRange: "6h"},
	{Key: "kumo_rocks_cache_total", Category: "Spool", Title: "RocksDB cache size", Description: "RocksDB spool block-cache size.", Unit: "bytes", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rocks_spool_cache_total)`, DefaultRange: "6h"},
	{Key: "kumo_rocks_compaction_pending", Category: "Spool", Title: "RocksDB pending compactions", Description: "Queued RocksDB compactions.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rocks_spool_compaction_pending)`, DefaultRange: "6h"},
	{Key: "kumo_rocks_bg_errors_rate", Category: "Spool", Title: "RocksDB background errors / sec", Description: "RocksDB background operation failures.", Unit: "events/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(rocks_spool_background_errors[$window]))`, DefaultRange: "6h"},

	// --- DNS / DKIM / DANE ---
	{Key: "kumo_dns_mx_resolve_rate", Category: "DNS/DKIM/DANE", Title: "MX resolutions / sec", Description: "Successful MX lookups per second.", Unit: "ops/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(dns_mx_resolve_status_ok[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_dns_mx_fail_rate", Category: "DNS/DKIM/DANE", Title: "MX resolve failures / sec", Description: "Failed MX lookups per second.", Unit: "ops/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(dns_mx_resolve_status_fail[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_dns_mx_inflight", Category: "DNS/DKIM/DANE", Title: "MX lookups in progress", Description: "Ongoing MX resolutions.", Unit: "count", Viz: WidgetVizStat, Instant: true,
		PromQLTemplate: `sum(dns_mx_resolve_in_progress)`, DefaultRange: "1h"},
	{Key: "kumo_dkim_sign_rate", Category: "DNS/DKIM/DANE", Title: "DKIM signatures / sec", Description: "DKIM signing operations per second.", Unit: "ops/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(dkim_signer_sign[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_dane_result_rate", Category: "DNS/DKIM/DANE", Title: "DANE results / sec", Description: "DANE validation outcomes per second.", Unit: "ops/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(dane_result_count[$window])) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"result"}, DefaultRange: "6h"},

	// --- Egress health ---
	{Key: "kumo_egress_suspended", Category: "Egress", Title: "Suspended egress sources", Description: "Egress sources currently health-suspended.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(egress_source_health_suspended) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"source"}, DefaultRange: "6h"},
	{Key: "kumo_egress_conn_failures_rate", Category: "Egress", Title: "Egress connection failures / sec", Description: "Egress-source connection failures per second.", Unit: "events/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(egress_source_connection_failures_total[$window])) $groupBy`, SupportsGroupBy: true, GroupByLabels: []string{"source"}, DefaultRange: "6h"},

	// --- Lua ---
	{Key: "kumo_lua_count", Category: "Lua", Title: "Active Lua contexts", Description: "Live Lua runtime contexts.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(lua_count)`, DefaultRange: "6h"},
	{Key: "kumo_lua_events_rate", Category: "Lua", Title: "Lua events / sec", Description: "Lua event-handler invocations per second.", Unit: "ops/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(lua_event_started[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_lua_event_latency_p95", Category: "Lua", Title: "Lua event latency p95", Description: "95th-percentile Lua event-handler duration.", Unit: "seconds", Viz: WidgetVizLine,
		PromQLTemplate: `histogram_quantile(0.95, sum(rate(lua_event_latency_bucket[$window])) by (le))`, DefaultRange: "6h"},

	// --- Logging ---
	{Key: "kumo_log_hook_backlog", Category: "Logging", Title: "Log hook backlog", Description: "Pending log-hook executions.", Unit: "count", Viz: WidgetVizLine,
		PromQLTemplate: `sum(log_hook_backlog_count)`, DefaultRange: "6h"},
	{Key: "kumo_log_dropped_rate", Category: "Logging", Title: "Dropped log events / sec", Description: "Log events dropped because the buffer was full.", Unit: "events/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(log_submit_full[$window]))`, DefaultRange: "6h"},

	// --- Maintenance ---
	{Key: "kumo_qmaint_runs_rate", Category: "Maintenance", Title: "Queue maintenance runs / sec", Description: "Scheduled-queue maintenance cycles per second.", Unit: "ops/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_qmaint_runs[$window]))`, DefaultRange: "6h"},
	{Key: "kumo_readyq_runs_rate", Category: "Maintenance", Title: "Ready-queue runs / sec", Description: "Ready-queue maintenance cycles per second.", Unit: "ops/s", Viz: WidgetVizLine,
		PromQLTemplate: `sum(rate(total_readyq_runs[$window]))`, DefaultRange: "6h"},

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
