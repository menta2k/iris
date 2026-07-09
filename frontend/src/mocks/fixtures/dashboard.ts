// Dashboard fixtures: scalar summary and generators for Prometheus-style mail-flow
// time-series and warmup delivery/bounce breakdowns. Generators are functions so
// the charts always span a fresh time window relative to the current dev session.

import type {
  DashboardSummary,
  DomainDeferredStat,
  MailClassStat,
  MailClassStats,
  MetricPoint,
  MetricsSeries,
  MetricsTimeseries,
  QueueTimeHistogram,
  RecipientDomainStat,
  RecipientDomainStats,
  WarmupDeliveryStat,
  WarmupDeliveryStats,
} from '../../types'
import { mailRecords } from './operations'

export const dashboardSummary: DashboardSummary = {
  serviceState: 'healthy',
  queuedMessages: '2114',
  recentMailEvents: '56',
  recentAuditEvents: '40',
  deferredInQueue: '318',
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

// ---- Mail volume by class / recipient domain -------------------------------

interface VolumeTally {
  count: number
  delivered: number
  bounced: number
  deferred: number
}

// Scale the handful of fixture records up to deployment-sized numbers, larger
// for the wider window.
function rangeScale(range: string): number {
  return range === '7d' ? 34 : 6
}

// Group the mail-record fixture by a key, counting total + terminal outcomes.
function tallyBy(keyOf: (r: (typeof mailRecords)[number]) => string): Map<string, VolumeTally> {
  const map = new Map<string, VolumeTally>()
  for (const r of mailRecords) {
    const key = keyOf(r)
    if (!key) continue
    const e = map.get(key) ?? { count: 0, delivered: 0, bounced: 0, deferred: 0 }
    e.count += 1
    if (r.status === 'delivered' || r.status === 'sent') e.delivered += 1
    else if (r.status === 'bounced') e.bounced += 1
    else if (r.status === 'deferred') e.deferred += 1
    map.set(key, e)
  }
  return map
}

function scaled(t: VolumeTally, scale: number): VolumeTally {
  return {
    count: t.count * scale,
    delivered: t.delivered * scale,
    bounced: t.bounced * scale,
    deferred: t.deferred * scale,
  }
}

export function mailClassStats(range: string): MailClassStats {
  const scale = rangeScale(range)
  const since = range === '7d' ? '7 days ago' : '24 hours ago'
  const rows: MailClassStat[] = [...tallyBy((r) => r.mailclass).entries()]
    .map(([mailclass, t]) => {
      const s = scaled(t, scale)
      return {
        mailclass,
        count: String(s.count),
        delivered: String(s.delivered),
        bounced: String(s.bounced),
        deferred: String(s.deferred),
      } satisfies MailClassStat
    })
    .sort((a, b) => Number(b.count) - Number(a.count))
  return { rows, range, since }
}

export function recipientDomainStats(range: string): RecipientDomainStats {
  const scale = rangeScale(range)
  const since = range === '7d' ? '7 days ago' : '24 hours ago'
  const rows: RecipientDomainStat[] = [...tallyBy((r) => r.recipientDomain).entries()]
    .map(([recipientDomain, t]) => {
      const s = scaled(t, scale)
      return {
        recipientDomain,
        count: String(s.count),
        delivered: String(s.delivered),
        bounced: String(s.bounced),
        deferred: String(s.deferred),
      } satisfies RecipientDomainStat
    })
    .sort((a, b) => Number(b.count) - Number(a.count))
    .slice(0, 10)
  return { rows, range, since }
}

// ---- Delivery queue-time histogram -----------------------------------------

// Bucket upper bounds (seconds) matching the backend histogram, plus the +Inf
// overflow. base is a realistic right-skewed distribution (most mail delivers
// fast, a long tail of deferred-then-delivered).
const QUEUE_BUCKET_BOUNDS = [0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1800, 3600, Infinity]
const QUEUE_BUCKET_BASE = [1200, 3400, 2600, 1500, 900, 500, 300, 180, 120, 80, 40, 20, 8, 4]

export function queueTimeHistogram(range: string, mailclass: string): QueueTimeHistogram {
  const rangeScale = range === '7d' ? 5 : range === '24h' ? 1 : range === '1h' ? 0.12 : 0.5
  const mcScale =
    mailclass === 'transactional'
      ? 0.5
      : mailclass === 'promo'
        ? 0.3
        : mailclass === 'newsletter'
          ? 0.2
          : 1
  const scale = rangeScale * mcScale
  let total = 0
  const buckets = QUEUE_BUCKET_BOUNDS.map((ub, i) => {
    const count = Math.round(QUEUE_BUCKET_BASE[i] * scale)
    total += count
    const isInf = !isFinite(ub)
    return {
      le: isInf ? '+Inf' : String(ub),
      // upperBound is unused by the UI (it derives bounds from `le`); 0 for +Inf
      // to avoid a non-finite value in JSON.
      upperBound: isInf ? 0 : ub,
      count: String(count),
    }
  })
  return {
    buckets,
    mailclasses: ['newsletter', 'promo', 'transactional'],
    totalCount: String(total),
    range,
    prometheusAvailable: true,
  }
}

function warmupDeferred(vi: number, di: number): number {
  return Math.round((200 + vi * 120 + di * 60) * (0.05 + 0.02 * di))
}

export function warmupDeliveryStats(range: string): WarmupDeliveryStats {
  const since = range === '7d' ? '7 days ago' : '24 hours ago'
  const rows: WarmupDeliveryStat[] = WARMUP_VMTAS.flatMap(([vmtaId, vmtaName], vi) =>
    WARMUP_DOMAINS.map((recipientDomain, di) => {
      const attempted = 200 + vi * 120 + di * 60
      const bounced = Math.round(attempted * (0.02 + 0.01 * (vi + di)))
      const deferred = warmupDeferred(vi, di)
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
  // Distinct messages deferred per domain: less than the per-VMTA sum, since a
  // message retries across ~2.5 VMTAs (demonstrates the dedup).
  const deferredByDomain: DomainDeferredStat[] = WARMUP_DOMAINS.map((recipientDomain, di) => {
    const perVmtaSum = WARMUP_VMTAS.reduce((acc, _v, vi) => acc + warmupDeferred(vi, di), 0)
    return { recipientDomain, messages: String(Math.round(perVmtaSum * 0.4)) }
  }).sort((a, b) => Number(b.messages) - Number(a.messages))
  return { rows, deferredByDomain, range, since }
}
