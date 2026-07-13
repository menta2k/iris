// Mock handlers for per-user custom dashboards: in-memory CRUD + set-default,
// the widget catalog (mirrors the Go biz.widgetCatalog), and widget-data (a
// deterministic timeseries generator that splits into series when grouped).

import type {
  MetricPoint,
  MetricsSeries,
  MetricsTimeseries,
  UserDashboard,
  WidgetCatalogEntry,
} from '../../types'
import { created, notFound, ok, type Route, type RouteCtx } from '../router'

// -- in-memory store --------------------------------------------------------

let seq = 1
function nextId(): string {
  return `dash-${seq++}`
}

function nowSec(): number {
  return Math.floor(Date.now() / 1000)
}

const store: UserDashboard[] = [
  {
    id: nextId(),
    name: 'My Overview',
    isDefault: true,
    widgetsJson: JSON.stringify([
      { id: 'w-seed-1', x: 0, y: 0, w: 6, h: 4, title: 'Delivered / sec', source: 'catalog', catalogKey: 'kumo_messages_delivered_rate', range: '6h', viz: 'line', groupBy: 'provider' },
      { id: 'w-seed-2', x: 6, y: 0, w: 6, h: 4, title: 'Received / sec', source: 'catalog', catalogKey: 'kumo_messages_received_rate', range: '6h', viz: 'area' },
      { id: 'w-seed-3', x: 0, y: 4, w: 3, h: 3, title: 'Messages in system', source: 'catalog', catalogKey: 'kumo_message_count', range: '1h', viz: 'stat', unit: 'count' },
    ]),
    createdAt: nowSec(),
    updatedAt: nowSec(),
  },
]

function find(id: string): UserDashboard | undefined {
  return store.find((d) => d.id === id)
}

// -- widget catalog (mirror of biz.widgetCatalog) ---------------------------

// Mirror of biz.widgetCatalog (backend/internal/biz/widget_catalog.go) so the
// builder shows the same catalog when running against the mock.
const g = (
  key: string,
  category: string,
  title: string,
  description: string,
  unit: string,
  viz: WidgetCatalogEntry['viz'],
  opts: Partial<Pick<WidgetCatalogEntry, 'supportsGroupBy' | 'groupByLabels' | 'defaultRange' | 'instant'>> = {},
): WidgetCatalogEntry => ({
  key,
  category,
  title,
  description,
  unit,
  viz,
  supportsGroupBy: opts.supportsGroupBy ?? false,
  groupByLabels: opts.groupByLabels,
  defaultRange: opts.defaultRange ?? '6h',
  instant: opts.instant ?? false,
})

const widgetCatalog: WidgetCatalogEntry[] = [
  // Messages
  g('kumo_messages_delivered_rate', 'Messages', 'Delivered / sec', 'Successful deliveries per second.', 'msg/s', 'line', { supportsGroupBy: true, groupByLabels: ['provider', 'source'] }),
  g('kumo_messages_received_rate', 'Messages', 'Received / sec', 'Inbound messages accepted by listeners per second.', 'msg/s', 'line'),
  g('kumo_messages_fail_rate', 'Messages', 'Failed (bounces) / sec', 'Permanent delivery failures per second.', 'msg/s', 'line', { supportsGroupBy: true, groupByLabels: ['provider', 'source'] }),
  g('kumo_messages_transfail_rate', 'Messages', 'Transient failures / sec', 'Retryable (transient) failures per second.', 'msg/s', 'line', { supportsGroupBy: true, groupByLabels: ['provider', 'source'] }),
  g('kumo_message_count', 'Messages', 'Messages in system', 'Messages currently spooled.', 'count', 'stat', { instant: true, defaultRange: '1h' }),
  // Queues
  g('kumo_scheduled_count', 'Queues', 'Scheduled queue depth', 'Messages awaiting their next delivery attempt.', 'count', 'line'),
  g('kumo_ready_count', 'Queues', 'Ready queue depth', 'Messages ready to send right now.', 'count', 'line'),
  g('kumo_queued_by', 'Queues', 'Queued by provider / pool', 'Queued messages, optionally grouped by provider or egress pool.', 'count', 'line', { supportsGroupBy: true, groupByLabels: ['provider', 'pool'] }),
  g('kumo_scheduled_queue_count', 'Queues', 'Active scheduled queues', 'Number of live scheduler queues.', 'count', 'stat', { instant: true, defaultRange: '1h' }),
  g('kumo_scheduled_by_domain', 'Queues', 'Scheduled by domain', 'Scheduled messages grouped by destination domain.', 'count', 'bar', { instant: true, defaultRange: '1h' }),
  g('kumo_scheduled_by_tenant', 'Queues', 'Scheduled by tenant', 'Scheduled messages grouped by tenant.', 'count', 'bar', { instant: true, defaultRange: '1h' }),
  g('kumo_ready_full_rate', 'Queues', 'Ready-queue-full events / sec', 'Rate at which the ready queue hit capacity.', 'events/s', 'line'),
  // Connections
  g('kumo_connection_count', 'Connections', 'Active connections', 'Open outbound SMTP connections.', 'count', 'line'),
  g('kumo_connection_by', 'Connections', 'Connections by provider / pool', 'Open connections, optionally grouped by provider or egress pool.', 'count', 'line', { supportsGroupBy: true, groupByLabels: ['provider', 'pool'] }),
  g('kumo_total_connections_rate', 'Connections', 'New connections / sec', 'Outbound connections opened per second.', 'conn/s', 'line'),
  g('kumo_connections_denied_rate', 'Connections', 'Connections denied / sec', 'Inbound connections rejected per second.', 'conn/s', 'line'),
  g('kumo_bind_failures_rate', 'Connections', 'Source bind failures / sec', 'Egress source binding errors per second.', 'events/s', 'line'),
  // Throttling
  g('kumo_throttle_message_rate', 'Throttling', 'Delayed: message-rate throttle / sec', 'Deliveries delayed by message-rate throttles.', 'events/s', 'line'),
  g('kumo_throttle_ready_full_rate', 'Throttling', 'Delayed: ready queue full / sec', 'Deliveries delayed because the ready queue was full.', 'events/s', 'line'),
  g('kumo_throttle_insert_ready_rate', 'Throttling', 'Delayed: throttle insert / sec', 'Deliveries delayed inserting into a throttled ready queue.', 'events/s', 'line'),
  // SMTP server
  g('kumo_smtp_rejections_rate', 'SMTP Server', 'SMTP rejections / sec', 'Inbound messages rejected, grouped by reason.', 'msg/s', 'line', { supportsGroupBy: true, groupByLabels: ['rejection_reason'] }),
  // Latency
  g('kumo_deliver_latency_p95', 'Latency', 'Delivery latency p95', '95th-percentile end-to-end delivery duration.', 'seconds', 'line'),
  g('kumo_queue_insert_latency_p95', 'Latency', 'Queue insert latency p95', '95th-percentile queue insertion duration.', 'seconds', 'line'),
  g('kumo_queue_resolve_latency_p95', 'Latency', 'Queue resolve latency p95', '95th-percentile queue resolution duration.', 'seconds', 'line'),
  g('kumo_ready_insert_latency_p95', 'Latency', 'Ready-queue insert latency p95', '95th-percentile ready-queue insertion duration.', 'seconds', 'line'),
  g('kumo_smtp_txn_duration_p95', 'Latency', 'SMTP transaction p95', '95th-percentile inbound SMTP transaction duration.', 'seconds', 'line'),
  g('kumo_smtp_process_data_p95', 'Latency', 'SMTP data processing p95', '95th-percentile message-processing duration.', 'seconds', 'line'),
  g('kumo_message_save_latency_p95', 'Latency', 'Spool save latency p95', '95th-percentile message persistence duration.', 'seconds', 'line'),
  // Resources
  g('kumo_memory_usage', 'Resources', 'Memory usage', 'kumod heap allocation.', 'bytes', 'line'),
  g('kumo_memory_usage_rust', 'Resources', 'Rust allocator memory', 'Rust allocator memory usage.', 'bytes', 'line'),
  g('kumo_memory_over_limit_rate', 'Resources', 'Memory over-limit events / sec', 'Rate of memory-limit-exceeded events.', 'events/s', 'line'),
  g('kumo_cpu_normalized', 'Resources', 'kumod CPU', 'kumod CPU usage relative to core count (0-1).', 'ratio', 'line'),
  g('kumo_system_cpu_normalized', 'Resources', 'System CPU', 'Host CPU usage relative to core count (0-1).', 'ratio', 'line'),
  g('kumo_thread_pool_parked', 'Resources', 'Parked threads', 'Idle worker threads.', 'count', 'line'),
  g('kumo_thread_pool_size', 'Resources', 'Thread pool size', 'Total worker threads.', 'count', 'line'),
  // Disk
  g('kumo_disk_free_percent', 'Disk', 'Disk free %', 'Minimum free disk space across spool volumes.', 'percent', 'gauge', { instant: true, defaultRange: '1h' }),
  g('kumo_disk_free_bytes', 'Disk', 'Disk free bytes', 'Minimum free disk space in bytes.', 'bytes', 'stat', { instant: true, defaultRange: '1h' }),
  g('kumo_disk_free_inodes_percent', 'Disk', 'Disk free inodes %', 'Minimum free inodes across spool volumes.', 'percent', 'gauge', { instant: true, defaultRange: '1h' }),
  // Spool
  g('kumo_data_resident_count', 'Spool', 'Resident message bodies', 'Message bodies held in memory.', 'count', 'line'),
  g('kumo_meta_resident_count', 'Spool', 'Resident metadata', 'Message metadata held in memory.', 'count', 'line'),
  g('kumo_rocks_cache_total', 'Spool', 'RocksDB cache size', 'RocksDB spool block-cache size.', 'bytes', 'line'),
  g('kumo_rocks_compaction_pending', 'Spool', 'RocksDB pending compactions', 'Queued RocksDB compactions.', 'count', 'line'),
  g('kumo_rocks_bg_errors_rate', 'Spool', 'RocksDB background errors / sec', 'RocksDB background operation failures.', 'events/s', 'line'),
  // DNS/DKIM/DANE
  g('kumo_dns_mx_resolve_rate', 'DNS/DKIM/DANE', 'MX resolutions / sec', 'Successful MX lookups per second.', 'ops/s', 'line'),
  g('kumo_dns_mx_fail_rate', 'DNS/DKIM/DANE', 'MX resolve failures / sec', 'Failed MX lookups per second.', 'ops/s', 'line'),
  g('kumo_dns_mx_inflight', 'DNS/DKIM/DANE', 'MX lookups in progress', 'Ongoing MX resolutions.', 'count', 'stat', { instant: true, defaultRange: '1h' }),
  g('kumo_dkim_sign_rate', 'DNS/DKIM/DANE', 'DKIM signatures / sec', 'DKIM signing operations per second.', 'ops/s', 'line'),
  g('kumo_dane_result_rate', 'DNS/DKIM/DANE', 'DANE results / sec', 'DANE validation outcomes per second.', 'ops/s', 'line', { supportsGroupBy: true, groupByLabels: ['result'] }),
  // Egress
  g('kumo_egress_suspended', 'Egress', 'Suspended egress sources', 'Egress sources currently health-suspended.', 'count', 'line', { supportsGroupBy: true, groupByLabels: ['source'] }),
  g('kumo_egress_conn_failures_rate', 'Egress', 'Egress connection failures / sec', 'Egress-source connection failures per second.', 'events/s', 'line', { supportsGroupBy: true, groupByLabels: ['source'] }),
  // Lua
  g('kumo_lua_count', 'Lua', 'Active Lua contexts', 'Live Lua runtime contexts.', 'count', 'line'),
  g('kumo_lua_events_rate', 'Lua', 'Lua events / sec', 'Lua event-handler invocations per second.', 'ops/s', 'line'),
  g('kumo_lua_event_latency_p95', 'Lua', 'Lua event latency p95', '95th-percentile Lua event-handler duration.', 'seconds', 'line'),
  // Logging
  g('kumo_log_hook_backlog', 'Logging', 'Log hook backlog', 'Pending log-hook executions.', 'count', 'line'),
  g('kumo_log_dropped_rate', 'Logging', 'Dropped log events / sec', 'Log events dropped because the buffer was full.', 'events/s', 'line'),
  // Maintenance
  g('kumo_qmaint_runs_rate', 'Maintenance', 'Queue maintenance runs / sec', 'Scheduled-queue maintenance cycles per second.', 'ops/s', 'line'),
  g('kumo_readyq_runs_rate', 'Maintenance', 'Ready-queue runs / sec', 'Ready-queue maintenance cycles per second.', 'ops/s', 'line'),
  // Iris
  g('iris_deliveries_rate', 'Iris', 'Deliveries / min', 'iris mail-flow deliveries per minute.', 'msg/min', 'area'),
  g('iris_receptions_rate', 'Iris', 'Receptions / min', 'iris messages received per minute.', 'msg/min', 'line'),
  g('iris_deferrals_rate', 'Iris', 'Deferrals / min', 'iris messages deferred per minute.', 'msg/min', 'line'),
  g('iris_mail_events_rate', 'Iris', 'Mail events / min', 'iris mail events per minute, optionally grouped by status, class, or recipient domain.', 'msg/min', 'line', { supportsGroupBy: true, groupByLabels: ['status', 'mailclass', 'recipient_domain'] }),
  g('iris_mail_by_domain', 'Iris', 'Mail by recipient domain (top)', 'Busiest recipient domains by mail volume per minute.', 'msg/min', 'bar'),
  g('iris_mail_by_class', 'Iris', 'Mail by class / min', 'Mail volume per minute grouped by mail class.', 'msg/min', 'line'),
  g('iris_bounces_rate', 'Iris', 'Bounces / min', 'iris bounces per minute.', 'msg/min', 'line', { supportsGroupBy: true, groupByLabels: ['type', 'mailclass'] }),
  g('iris_vmta_events_rate', 'Iris', 'VMTA events / min', 'Outbound events per minute, optionally grouped by VMTA or status.', 'msg/min', 'line', { supportsGroupBy: true, groupByLabels: ['vmta', 'status'] }),
  g('iris_webhook_rate', 'Iris', 'Webhook executions / min', 'Webhook deliveries per minute, optionally grouped by webhook or result.', 'ops/min', 'line', { supportsGroupBy: true, groupByLabels: ['webhook', 'result'] }),
  g('iris_queue_time_p50', 'Iris', 'Queue time p50', 'Median time from reception to delivery.', 'seconds', 'line'),
  g('iris_queue_time_p95', 'Iris', 'Queue time p95', '95th-percentile time from reception to delivery.', 'seconds', 'line'),
  g('iris_cpu_percent', 'Iris', 'Host CPU %', 'iris host CPU utilization.', 'percent', 'gauge', { instant: true, defaultRange: '1h' }),
  g('iris_cpu_percent_trend', 'Iris', 'Host CPU % (trend)', 'iris host CPU utilization over time.', 'percent', 'area'),
  g('iris_memory_percent', 'Iris', 'Host memory %', 'iris host memory utilization.', 'percent', 'gauge', { instant: true, defaultRange: '1h' }),
  g('iris_memory_percent_trend', 'Iris', 'Host memory % (trend)', 'iris host memory utilization over time.', 'percent', 'area'),
  g('iris_memory_used_bytes', 'Iris', 'Host memory used', 'iris host memory used in bytes.', 'bytes', 'line'),
  g('iris_disk_used_percent', 'Iris', 'Disk used % (max)', 'Highest filesystem usage across monitored mounts.', 'percent', 'gauge', { instant: true, defaultRange: '1h' }),
  g('iris_disk_used_by_path', 'Iris', 'Disk used % by mount', 'Filesystem usage grouped by mount path.', 'percent', 'bar', { instant: true, defaultRange: '1h' }),
]

// -- widget-data generator --------------------------------------------------

function specForRange(range: string): { stepSeconds: number; points: number } {
  switch (range) {
    case '1h':
      return { stepSeconds: 5 * 60, points: 12 }
    case '24h':
      return { stepSeconds: 2 * 3600, points: 12 }
    case '7d':
      return { stepSeconds: 12 * 3600, points: 14 }
    case '6h':
    default:
      return { stepSeconds: 30 * 60, points: 12 }
  }
}

function wiggle(seed: number, index: number): number {
  const s = Math.sin((seed + index) * 1.3) + Math.sin((seed + index) * 0.37)
  return Math.abs(s) / 2
}

function buildSeries(key: string, label: string, base: number, amp: number, seed: number, spec: { stepSeconds: number; points: number }, now: number): MetricsSeries {
  const points: MetricPoint[] = []
  for (let i = 0; i < spec.points; i += 1) {
    const timestamp = Math.floor((now - (spec.points - 1 - i) * spec.stepSeconds * 1000) / 1000)
    points.push({ timestamp, value: Math.round(base + amp * wiggle(seed, i)) })
  }
  return { key, label, points }
}

const GROUP_MEMBERS: Record<string, string[]> = {
  provider: ['gmail', 'yahoo', 'microsoft'],
  pool: ['pool-a', 'pool-b'],
  source: ['ip-1', 'ip-2'],
  result: ['secure', 'insecure', 'tempfail'],
  rejection_reason: ['relay-denied', 'rate-limited', 'bad-recipient'],
  domain: ['gmail.com', 'yahoo.com', 'outlook.com', 'example.com'],
  tenant: ['tenant-a', 'tenant-b', 'tenant-c'],
  status: ['sent', 'received', 'deferred', 'bounced'],
  mailclass: ['default', 'transactional', 'bulk'],
  recipient_domain: ['gmail.com', 'yahoo.com', 'outlook.com'],
  type: ['hard', 'soft', 'dsn'],
  vmta: ['vmta-1', 'vmta-2', 'vmta-3'],
  webhook: ['orders', 'alerts'],
}

function widgetData(ctx: RouteCtx): MetricsTimeseries {
  const range = ctx.query.range || '6h'
  const groupBy = ctx.query.groupBy || ''
  const spec = specForRange(range)
  const now = Date.now()

  let series: MetricsSeries[]
  if (groupBy && GROUP_MEMBERS[groupBy]) {
    series = GROUP_MEMBERS[groupBy].map((member, i) =>
      buildSeries(member, member, 120 - i * 30, 60, i + 2, spec, now),
    )
  } else {
    series = [buildSeries('value', 'value', 260, 140, 1, spec, now)]
  }
  return { series, range, stepSeconds: spec.stepSeconds, prometheusAvailable: true }
}

// -- routes -----------------------------------------------------------------

export const dashboardsRoutes: Route[] = [
  { method: 'GET', pattern: '/dashboards', handler: () => ok({ dashboards: store }) },
  {
    method: 'POST',
    pattern: '/dashboards',
    handler: (ctx) => {
      const body = (ctx.body ?? {}) as Record<string, unknown>
      const makeDefault = Boolean(body.makeDefault)
      if (makeDefault) store.forEach((d) => (d.isDefault = false))
      const dash: UserDashboard = {
        id: nextId(),
        name: String(body.name ?? 'Untitled'),
        isDefault: makeDefault,
        widgetsJson: String(body.widgetsJson ?? '[]'),
        createdAt: nowSec(),
        updatedAt: nowSec(),
      }
      store.push(dash)
      return created(dash)
    },
  },
  {
    method: 'PUT',
    pattern: '/dashboards/:id',
    handler: (ctx) => {
      const dash = find(ctx.params.id)
      if (!dash) return notFound('dashboard not found')
      const body = (ctx.body ?? {}) as Record<string, unknown>
      dash.name = String(body.name ?? dash.name)
      dash.widgetsJson = String(body.widgetsJson ?? dash.widgetsJson)
      dash.updatedAt = nowSec()
      return ok(dash)
    },
  },
  {
    method: 'DELETE',
    pattern: '/dashboards/:id',
    handler: (ctx) => {
      const idx = store.findIndex((d) => d.id === ctx.params.id)
      if (idx === -1) return notFound('dashboard not found')
      store.splice(idx, 1)
      return ok({})
    },
  },
  {
    method: 'POST',
    pattern: '/dashboards/:id:set-default',
    handler: (ctx) => {
      const dash = find(ctx.params.id)
      if (!dash) return notFound('dashboard not found')
      store.forEach((d) => (d.isDefault = d.id === dash.id))
      dash.updatedAt = nowSec()
      return ok(dash)
    },
  },
  { method: 'GET', pattern: '/dashboard/widget-catalog', handler: () => ok({ widgets: widgetCatalog }) },
  { method: 'GET', pattern: '/dashboard/widget-data', handler: (ctx) => ok(widgetData(ctx)) },
]
