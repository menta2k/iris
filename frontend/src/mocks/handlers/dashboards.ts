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

const widgetCatalog: WidgetCatalogEntry[] = [
  { key: 'kumo_messages_delivered_rate', category: 'Messages', title: 'Delivered / sec', description: 'Successful deliveries per second.', unit: 'msg/s', viz: 'line', supportsGroupBy: true, groupByLabels: ['provider', 'pool', 'source'], defaultRange: '6h', instant: false },
  { key: 'kumo_messages_received_rate', category: 'Messages', title: 'Received / sec', description: 'Inbound messages received per second.', unit: 'msg/s', viz: 'line', supportsGroupBy: false, defaultRange: '6h', instant: false },
  { key: 'kumo_messages_fail_rate', category: 'Messages', title: 'Failed / sec', description: 'Permanent delivery failures (bounces) per second.', unit: 'msg/s', viz: 'line', supportsGroupBy: true, groupByLabels: ['provider', 'pool', 'source'], defaultRange: '6h', instant: false },
  { key: 'kumo_message_count', category: 'Messages', title: 'Messages in system', description: 'Messages currently held in memory.', unit: 'count', viz: 'stat', supportsGroupBy: false, defaultRange: '1h', instant: true },
  { key: 'kumo_scheduled_count', category: 'Queues', title: 'Scheduled queue depth', description: 'Messages awaiting their next delivery attempt.', unit: 'count', viz: 'line', supportsGroupBy: true, groupByLabels: ['provider', 'pool'], defaultRange: '6h', instant: false },
  { key: 'kumo_ready_count', category: 'Queues', title: 'Ready queue depth', description: 'Messages ready to send right now.', unit: 'count', viz: 'line', supportsGroupBy: true, groupByLabels: ['provider', 'pool'], defaultRange: '6h', instant: false },
  { key: 'kumo_connection_count', category: 'Connections', title: 'Active connections', description: 'Open outbound SMTP connections.', unit: 'count', viz: 'line', supportsGroupBy: true, groupByLabels: ['provider', 'pool'], defaultRange: '6h', instant: false },
  { key: 'kumo_memory_usage', category: 'Resources', title: 'Memory usage', description: 'kumod process memory consumption.', unit: 'bytes', viz: 'line', supportsGroupBy: false, defaultRange: '6h', instant: false },
  { key: 'iris_deliveries_rate', category: 'Iris', title: 'Deliveries / min', description: 'iris mail-flow deliveries per minute.', unit: 'msg/min', viz: 'area', supportsGroupBy: false, defaultRange: '6h', instant: false },
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
