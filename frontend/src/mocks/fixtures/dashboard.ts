// Dashboard fixtures: scalar summary and generators for Prometheus-style mail-flow
// time-series and warmup delivery/bounce breakdowns. Generators are functions so
// the charts always span a fresh time window relative to the current dev session.

import type {
  DashboardSummary,
  MetricPoint,
  MetricsSeries,
  MetricsTimeseries,
  WarmupDeliveryStat,
  WarmupDeliveryStats,
} from '../../types'

export const dashboardSummary: DashboardSummary = {
  serviceState: 'healthy',
  queuedMessages: '2114',
  recentMailEvents: '56',
  recentAuditEvents: '40',
}

interface RangeSpec {
  stepSeconds: number
  points: number
}

function specForRange(range: string): RangeSpec {
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

// Smooth pseudo-random walk anchored to the series key + index, so curves look
// organic without depending on Math.random (deterministic per render).
function wiggle(seed: number, index: number): number {
  const s = Math.sin((seed + index) * 1.3) + Math.sin((seed + index) * 0.37)
  return Math.abs(s) / 2
}

function buildSeries(key: string, label: string, base: number, amp: number, seed: number, spec: RangeSpec, now: number): MetricsSeries {
  const points: MetricPoint[] = []
  for (let i = 0; i < spec.points; i += 1) {
    const timestamp = Math.floor((now - (spec.points - 1 - i) * spec.stepSeconds * 1000) / 1000)
    const value = Math.round(base + amp * wiggle(seed, i))
    points.push({ timestamp, value })
  }
  return { key, label, points }
}

export function metricsTimeseries(range: string): MetricsTimeseries {
  const spec = specForRange(range)
  const now = Date.now()
  const series: MetricsSeries[] = [
    buildSeries('received', 'Received', 320, 180, 1, spec, now),
    buildSeries('delivered', 'Delivered', 300, 170, 2, spec, now),
    buildSeries('deferred', 'Deferred', 24, 30, 3, spec, now),
    buildSeries('bounced', 'Bounced', 12, 18, 4, spec, now),
  ]
  return { series, range, stepSeconds: spec.stepSeconds, prometheusAvailable: true }
}

const WARMUP_DOMAINS = ['gmail.com', 'yahoo.com', 'outlook.com']
const WARMUP_VMTAS: Array<[string, string]> = [
  ['vmta6', 'warmup-1'],
  ['vmta3', 'promo-3'],
  ['vmta1', 'promo-1'],
]

export function warmupDeliveryStats(range: string): WarmupDeliveryStats {
  const since = range === '7d' ? '7 days ago' : '24 hours ago'
  const rows: WarmupDeliveryStat[] = WARMUP_VMTAS.flatMap(([vmtaId, vmtaName], vi) =>
    WARMUP_DOMAINS.map((recipientDomain, di) => {
      const attempted = 200 + vi * 120 + di * 60
      const bounced = Math.round(attempted * (0.02 + 0.01 * (vi + di)))
      const deferred = Math.round(attempted * (0.05 + 0.02 * di))
      const sent = attempted - bounced
      return {
        vmtaId,
        vmtaName,
        recipientDomain,
        sent: String(sent),
        bounced: String(bounced),
        deferred: String(deferred),
        attempted: String(attempted),
        deliveryRate: Number((sent / attempted).toFixed(3)),
        bounceRate: Number((bounced / attempted).toFixed(3)),
      } satisfies WarmupDeliveryStat
    }),
  )
  return { rows, range, since }
}
