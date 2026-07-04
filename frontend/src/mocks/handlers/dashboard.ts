// Dashboard handlers: scalar summary and the Prometheus-backed mail-flow and
// warmup time-series. The `range` query param is interpolated into the path by
// the service, so it arrives here as a normal query string.

import {
  dashboardSummary,
  mailClassStats,
  metricsTimeseries,
  queueTimeHistogram,
  recipientDomainStats,
  warmupDeliveryStats,
} from '../fixtures/dashboard'
import { ok, type Route } from '../router'

export const dashboardRoutes: Route[] = [
  { method: 'GET', pattern: '/dashboard/summary', handler: () => ok(dashboardSummary) },
  {
    method: 'GET',
    pattern: '/dashboard/warmup-stats',
    handler: (ctx) => ok(warmupDeliveryStats(ctx.query.range || '24h')),
  },
  {
    method: 'GET',
    pattern: '/dashboard/mailclass-stats',
    handler: (ctx) => ok(mailClassStats(ctx.query.range || '24h')),
  },
  {
    method: 'GET',
    pattern: '/dashboard/recipient-domain-stats',
    handler: (ctx) => ok(recipientDomainStats(ctx.query.range || '24h')),
  },
  {
    method: 'GET',
    pattern: '/dashboard/metrics',
    handler: (ctx) => ok(metricsTimeseries(ctx.query.range || '6h')),
  },
  {
    method: 'GET',
    pattern: '/dashboard/queue-time-histogram',
    handler: (ctx) => ok(queueTimeHistogram(ctx.query.range || '6h', ctx.query.mailclass || '')),
  },
]
