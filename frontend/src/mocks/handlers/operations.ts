// Operations/monitoring handlers: mail records (filterable + paged), bounces,
// feedback reports, queues + queue actions, service control, worker errors,
// DMARC stats/reports/domains, and the domain bounce-readiness check.

import type {
  MailRecord,
} from '../../types'
import { all, paged, updateRow } from '../db'
import { dmarcDomains, dmarcReports } from '../fixtures/operations'
import { notFound, ok, type Route } from '../router'

function includes(haystack: string | undefined, needle: string): boolean {
  return !!haystack && haystack.toLowerCase().includes(needle.toLowerCase())
}

function mailRecordFilter(query: Record<string, string>): ((r: MailRecord) => boolean) | undefined {
  const filters: Array<(r: MailRecord) => boolean> = []
  if (query.mailclass) filters.push((r) => r.mailclass === query.mailclass)
  if (query.sender) filters.push((r) => includes(r.sender, query.sender))
  if (query.from) filters.push((r) => includes(r.fromHeader, query.from))
  if (query.recipient) filters.push((r) => includes(r.recipient, query.recipient))
  if (query.vmta_id) filters.push((r) => r.vmtaId === query.vmta_id)
  if (query.status) filters.push((r) => r.status === query.status)
  if (query.record_type) filters.push((r) => r.recordType === query.record_type)
  if (filters.length === 0) return undefined
  return (r) => filters.every((fn) => fn(r))
}

function domainCheck(domain: string) {
  const isExample = domain.endsWith('example.net') || domain.endsWith('example.com')
  return {
    domain,
    items: [
      { name: 'MX record', status: isExample ? 'pass' : 'fail', detail: isExample ? '1 MX target resolved' : 'No MX records found', records: isExample ? ['10 mta1.example.net.'] : [] },
      { name: 'SPF', status: isExample ? 'pass' : 'warn', detail: isExample ? 'v=spf1 include:_spf.example.net -all' : 'SPF not found', records: isExample ? ['"v=spf1 include:_spf.example.net -all"'] : [] },
      { name: 'DKIM (iris selector)', status: domain.endsWith('example.net') ? 'pass' : 'warn', detail: domain.endsWith('example.net') ? 'Public key published' : 'No DKIM record for selector', records: domain.endsWith('example.net') ? ['"v=DKIM1; k=rsa; p=MIIB..."'] : [] },
      { name: 'DMARC', status: isExample ? 'pass' : 'fail', detail: isExample ? 'p=quarantine; pct=100' : 'No DMARC record', records: isExample ? ['"v=DMARC1; p=quarantine; rua=mailto:dmarc@example.net"'] : [] },
      { name: 'PTR (reverse DNS)', status: isExample ? 'pass' : 'warn', detail: isExample ? 'Forward-confirmed' : 'PTR mismatch' },
    ],
  }
}

export const operationsRoutes: Route[] = [
  // ---- Mail records ----
  {
    method: 'GET',
    pattern: '/mail-records',
    handler: (ctx) => ok(paged(all('mailRecords'), ctx.query, { filter: mailRecordFilter(ctx.query), defaultSize: 25 })),
  },

  // ---- Bounces ----
  { method: 'GET', pattern: '/bounces', handler: (ctx) => ok(paged(all('bounces'), ctx.query)) },

  // ---- Feedback reports ----
  { method: 'GET', pattern: '/feedback-reports', handler: (ctx) => ok(paged(all('feedbackReports'), ctx.query)) },

  // ---- Queues ----
  { method: 'GET', pattern: '/queues', handler: () => ok({ items: all('queues'), page: {} }) },
  {
    method: 'POST',
    pattern: '/queues:action',
    handler: (ctx) => {
      const body = ctx.body as { action: 'suspend' | 'resume' | 'bounce'; domain: string; reason?: string }
      const queue = all('queues').find((q) => q.domain === body.domain)
      if (!queue) return notFound(`Queue for ${body.domain} not found`)
      if (body.action === 'suspend') {
        updateRow('queues', body.domain, { suspended: true, suspendReason: body.reason ?? 'Suspended via UI' })
      } else if (body.action === 'resume') {
        updateRow('queues', body.domain, { suspended: false, suspendReason: undefined })
      }
      const verb = body.action === 'suspend' ? 'suspended' : body.action === 'resume' ? 'resumed' : 'bounced'
      return ok({ status: 'ok', summary: `Queue ${body.domain} ${verb}` })
    },
  },

  // ---- Service control ----
  {
    method: 'POST',
    pattern: '/kumomta:service-control',
    handler: (ctx) => {
      const body = ctx.body as { operation: string }
      return ok({ id: `svc_${Date.now().toString(36)}`, operation: body.operation, status: 'completed' })
    },
  },

  // ---- Worker error logs ----
  {
    method: 'GET',
    pattern: '/worker-error-logs',
    handler: (ctx) => {
      const level = ctx.query.level
      const worker = ctx.query.worker
      const filter =
        level || worker
          ? (w: { level: string; worker: string }) => (!level || w.level === level) && (!worker || w.worker === worker)
          : undefined
      return ok(paged(all('workerErrors'), ctx.query, { filter }))
    },
  },

  // ---- DMARC ----
  {
    method: 'GET',
    pattern: '/dmarc/stats',
    handler: (ctx) => {
      const domains = ctx.query.domain ? [ctx.query.domain] : dmarcDomains
      const total = 18432
      return ok({
        totalMessages: total,
        dmarcPass: Math.round(total * 0.97),
        spfPass: Math.round(total * 0.95),
        dkimPass: Math.round(total * 0.96),
        dispositions: [
          { label: 'none', count: Math.round(total * 0.96) },
          { label: 'quarantine', count: Math.round(total * 0.03) },
          { label: 'reject', count: Math.round(total * 0.01) },
        ],
        topSources: [
          { ip: '209.85.220.41', total: 8200, pass: 8050, fail: 150 },
          { ip: '74.6.231.20', total: 4100, pass: 3990, fail: 110 },
          { ip: '40.92.38.5', total: 2300, pass: 2260, fail: 40 },
        ],
        domains: domains.map((domain, i) => ({ domain, messages: 6000 - i * 1500, pass: (6000 - i * 1500) - 120 })),
        series: Array.from({ length: 14 }, (_, i) => ({ date: new Date(Date.now() - (13 - i) * 86400000).toISOString().slice(0, 10), messages: 1200 + Math.round(Math.sin(i) * 200) + 100, pass: 1180 + Math.round(Math.sin(i) * 180) })),
      })
    },
  },
  {
    method: 'GET',
    pattern: '/dmarc/reports',
    handler: (ctx) =>
      ok(paged(ctx.query.domain ? dmarcReports.filter((r) => r.domain === ctx.query.domain) : dmarcReports, ctx.query)),
  },
  { method: 'GET', pattern: '/dmarc/domains', handler: () => ok({ domains: dmarcDomains }) },

  // ---- Domain bounce-readiness check ----
  { method: 'GET', pattern: '/domain-check/:domain', handler: (ctx) => ok(domainCheck(ctx.params.domain)) },
]
