// Operations/monitoring handlers: mail records (filterable + paged), bounces,
// feedback reports, queues + queue actions, service control, worker errors,
// DMARC stats/reports/domains, and the domain bounce-readiness check.

import type {
  Bounce,
  MailRecord,
} from '../../types'
import { all, paged, updateWhere } from '../db'
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
  if (query.status)
    filters.push((r) => (r.status ?? '').toLowerCase() === query.status.toLowerCase())
  if (query.record_type) filters.push((r) => r.recordType === query.record_type)
  if (query.diagnostic) filters.push((r) => includes(r.diagnostic, query.diagnostic))
  // Time range: RFC3339 bounds, matching the backend's from_time/to_time.
  if (query.from_time) {
    const t = Date.parse(query.from_time)
    if (!Number.isNaN(t)) filters.push((r) => Date.parse(r.eventTime) >= t)
  }
  if (query.to_time) {
    const t = Date.parse(query.to_time)
    if (!Number.isNaN(t)) filters.push((r) => Date.parse(r.eventTime) <= t)
  }
  if (filters.length === 0) return undefined
  return (r) => filters.every((fn) => fn(r))
}

// Mirrors the backend's ListBounces filters (substring recipient/classification,
// exact mailclass/type/state, RFC3339 time bounds).
function bounceFilter(query: Record<string, string>): ((b: Bounce) => boolean) | undefined {
  const filters: Array<(b: Bounce) => boolean> = []
  if (query.recipient) filters.push((b) => includes(b.recipient, query.recipient))
  if (query.mailclass) filters.push((b) => b.mailclass === query.mailclass)
  if (query.bounce_type)
    filters.push((b) => (b.bounceType ?? '').toLowerCase() === query.bounce_type.toLowerCase())
  if (query.classification) filters.push((b) => includes(b.classification, query.classification))
  if (query.processing_state)
    filters.push((b) => (b.processingState ?? '').toLowerCase() === query.processing_state.toLowerCase())
  if (query.from_time) {
    const t = Date.parse(query.from_time)
    if (!Number.isNaN(t)) filters.push((b) => Date.parse(b.eventTime) >= t)
  }
  if (query.to_time) {
    const t = Date.parse(query.to_time)
    if (!Number.isNaN(t)) filters.push((b) => Date.parse(b.eventTime) <= t)
  }
  if (filters.length === 0) return undefined
  return (b) => filters.every((fn) => fn(b))
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

const RETRY_INTERVAL_MS = 20 * 60 * 1000 // KumoMTA default retry_interval (20m)
const MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000 // default max_age (7d)

// nextDeliveryAttempt mirrors the backend estimator over the mock records that
// share a message id: exponential backoff (20m, doubling) up to a 7d max age.
function nextDeliveryAttempt(messageId: string) {
  const events = all('mailRecords').filter((r) => r.messageId === messageId)
  let created = Infinity
  let lastDeferral = -Infinity
  let attempts = 0
  let terminal = false
  for (const e of events) {
    const t = new Date(e.eventTime).getTime()
    const rt = (e.recordType || '').toLowerCase()
    const st = (e.status || '').toLowerCase()
    if (rt === 'reception' || st === 'received') created = Math.min(created, t)
    else if (rt === 'transientfailure' || st === 'deferred') { attempts++; lastDeferral = Math.max(lastDeferral, t) }
    else if (['delivery', 'bounce', 'expiration'].includes(rt) || ['sent', 'delivered', 'bounced'].includes(st)) terminal = true
  }
  if (terminal || attempts === 0 || lastDeferral < 0) {
    return { deferred: false, attempts, remainingAttempts: 0, willExpire: false }
  }
  let interval = RETRY_INTERVAL_MS * 2 ** (attempts - 1)
  const nextAttempt = lastDeferral + interval
  const expiresAt = Number.isFinite(created) ? created + MAX_AGE_MS : NaN
  let remaining = 0
  let final = 0
  if (Number.isFinite(expiresAt)) {
    let at = lastDeferral
    let iv = interval
    while (remaining < 100000) {
      at += iv
      if (at > expiresAt) break
      remaining++
      final = at
      iv = Math.min(iv * 2, MAX_AGE_MS)
    }
  }
  const isoOrUndef = (ms: number) => (Number.isFinite(ms) && ms > 0 ? new Date(ms).toISOString() : undefined)
  return {
    deferred: true,
    attempts,
    lastAttempt: isoOrUndef(lastDeferral),
    nextAttempt: isoOrUndef(nextAttempt),
    remainingAttempts: remaining,
    finalAttempt: isoOrUndef(final),
    willExpire: Number.isFinite(expiresAt) && nextAttempt > expiresAt,
    expiresAt: isoOrUndef(expiresAt),
    interval: `${Math.round(interval / 60000)}m`,
  }
}

export const operationsRoutes: Route[] = [
  // ---- Mail records ----
  {
    method: 'GET',
    pattern: '/mail-records',
    handler: (ctx) => ok(paged(all('mailRecords'), ctx.query, { filter: mailRecordFilter(ctx.query), defaultSize: 25 })),
  },
  {
    method: 'GET',
    pattern: '/mail-records/:messageId/next-attempt',
    handler: (ctx) => ok(nextDeliveryAttempt(decodeURIComponent(ctx.params.messageId))),
  },

  // ---- Bounces ----
  {
    method: 'GET',
    pattern: '/bounces',
    handler: (ctx) => ok(paged(all('bounces'), ctx.query, { filter: bounceFilter(ctx.query) })),
  },
  {
    method: 'GET',
    pattern: '/dsn-messages',
    handler: (ctx) => {
      const recipient = (ctx.query.recipient || '').toString().toLowerCase()
      const match = all('bounces').some(
        (b) => (b as { recipient: string; bounceType: string }).recipient.toLowerCase() === recipient
          && (b as { bounceType: string }).bounceType === 'dsn',
      )
      if (!recipient || !match) return ok({ items: [] })
      return ok({
        items: [
          {
            id: `dsn_${recipient}`,
            messageId: 'a1b2c3d4e5f60718293a4b5c6d7e8f90',
            receivedAt: new Date().toISOString(),
            rawMessage: [
              'Return-Path: <>',
              'From: Mail Delivery System <MAILER-DAEMON@mx.example.com>',
              `To: <${recipient}>`,
              'Subject: Undelivered Mail Returned to Sender',
              'Content-Type: multipart/report; report-type=delivery-status;',
              '',
              'This is the mail system at host mx.example.com.',
              '',
              "I'm sorry to have to inform you that your message could not",
              'be delivered to one or more recipients.',
              '',
              '--- The following addresses had permanent fatal errors ---',
              `<${recipient}>`,
              '    (reason: 550 5.1.1 <user> User unknown; rejecting)',
              '',
              '--- Delivery report ---',
              'Action: failed',
              'Status: 5.1.1',
              'Diagnostic-Code: smtp; 550 5.1.1 recipient rejected',
            ].join('\n'),
          },
        ],
      })
    },
  },

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
      // Queues have no id — they are keyed by domain.
      if (body.action === 'suspend') {
        updateWhere('queues', (q) => q.domain === body.domain, {
          suspended: true,
          suspendReason: body.reason ?? 'Suspended via UI',
        })
      } else if (body.action === 'resume') {
        updateWhere('queues', (q) => q.domain === body.domain, {
          suspended: false,
          suspendReason: undefined,
        })
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
      // Reporters (org_name) with their share of the grand total. The breakdown
      // always lists every reporter; selecting one drills the summary/charts
      // down to that reporter's slice.
      const REPORTERS = [
        { reporter: 'google.com', share: 0.52 },
        { reporter: 'Yahoo', share: 0.23 },
        { reporter: 'Enterprise Outlook', share: 0.14 },
        { reporter: 'Mail.Ru', share: 0.07 },
        { reporter: 'AMAZON-SES', share: 0.04 },
      ]
      const grandTotal = 18432
      const selShare = ctx.query.reporter
        ? (REPORTERS.find((r) => r.reporter === ctx.query.reporter)?.share ?? 0)
        : 1
      const total = Math.round(grandTotal * selShare)
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
          { ip: '209.85.220.41', total: Math.round(8200 * selShare), pass: Math.round(8050 * selShare), fail: Math.round(150 * selShare) },
          { ip: '74.6.231.20', total: Math.round(4100 * selShare), pass: Math.round(3990 * selShare), fail: Math.round(110 * selShare) },
          { ip: '40.92.38.5', total: Math.round(2300 * selShare), pass: Math.round(2260 * selShare), fail: Math.round(40 * selShare) },
        ],
        domains: domains.map((domain, i) => {
          const messages = Math.round((6000 - i * 1500) * selShare)
          return { domain, messages, pass: Math.max(0, messages - Math.round(120 * selShare)) }
        }),
        reporters: REPORTERS.map(({ reporter, share }) => {
          const messages = Math.round(grandTotal * share)
          return { reporter, messages, pass: Math.round(messages * 0.97) }
        }),
        series: Array.from({ length: 14 }, (_, i) => ({
          date: new Date(Date.now() - (13 - i) * 86400000).toISOString().slice(0, 10),
          messages: Math.round((1200 + Math.round(Math.sin(i) * 200) + 100) * selShare),
          pass: Math.round((1180 + Math.round(Math.sin(i) * 180)) * selShare),
        })),
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
