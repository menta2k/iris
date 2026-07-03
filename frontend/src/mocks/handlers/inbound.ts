// Inbound handlers: inbound routes (CRUD), Rspamd scan results (list), and
// registered feedback loops (CRUD).

import type { InboundRoute } from '../../types'
import { all, createRow, genId, paged, removeRow, updateRow } from '../db'
import { noContent, notFound, ok, type Route } from '../router'

type InboundRouteBody = {
  name: string
  match_type: string
  match_value: string
  action: string
  priority: number
  status: string
  spam_scan: string
  forward_host: string
  forward_port: number
  forward_tls: string
  maildir_path: string
  destination_url: string
  timeout_seconds: number
  secret_ref: string
}

function toRoute(id: string, body: InboundRouteBody): InboundRoute {
  return {
    id,
    name: body.name,
    matchType: body.match_type,
    matchValue: body.match_value,
    action: body.action,
    priority: body.priority,
    status: body.status,
    spamScan: body.spam_scan,
    forwardHost: body.forward_host,
    forwardPort: body.forward_port,
    forwardTls: body.forward_tls,
    maildirPath: body.maildir_path,
    destinationUrl: body.destination_url,
    timeoutSeconds: body.timeout_seconds,
  }
}

export const inboundHandlers: Route[] = [
  { method: 'GET', pattern: '/inbound-routes', handler: (ctx) => ok(paged(all('inboundRoutes'), ctx.query)) },
  { method: 'POST', pattern: '/inbound-routes', handler: (ctx) => ok(createRow('inboundRoutes', toRoute(genId('ibr'), ctx.body as InboundRouteBody))) },
  {
    method: 'PUT',
    pattern: '/inbound-routes/:id',
    handler: (ctx) => {
      const updated = updateRow('inboundRoutes', ctx.params.id, toRoute(ctx.params.id, ctx.body as InboundRouteBody))
      return updated ? ok(updated) : notFound('Inbound route not found')
    },
  },
  {
    method: 'DELETE',
    pattern: '/inbound-routes/:id',
    handler: (ctx) => (removeRow('inboundRoutes', ctx.params.id) ? ok({ ok: true }) : notFound('Inbound route not found')),
  },

  { method: 'GET', pattern: '/rspamd-results', handler: (ctx) => ok(paged(all('rspamdResults'), ctx.query)) },

  // ---- Feedback loops ----
  { method: 'GET', pattern: '/feedback-loops', handler: (ctx) => ok(paged(all('feedbackLoops'), ctx.query)) },
  {
    method: 'POST',
    pattern: '/feedback-loops',
    handler: (ctx) => {
      const body = ctx.body as { domain: string; feedback_address: string; forward_address: string; status: string }
      return ok(createRow('feedbackLoops', {
        id: genId('fbl'),
        domain: body.domain,
        feedbackAddress: body.feedback_address,
        forwardAddress: body.forward_address,
        status: body.status || 'awaiting_approval',
      }))
    },
  },
  {
    method: 'PUT',
    pattern: '/feedback-loops/:id',
    handler: (ctx) => {
      const body = ctx.body as { domain: string; feedback_address: string; forward_address: string; status: string }
      const updated = updateRow('feedbackLoops', ctx.params.id, { domain: body.domain, feedbackAddress: body.feedback_address, forwardAddress: body.forward_address, status: body.status })
      return updated ? ok(updated) : notFound('Feedback loop not found')
    },
  },
  {
    method: 'DELETE',
    pattern: '/feedback-loops/:id',
    handler: (ctx) => {
      removeRow('feedbackLoops', ctx.params.id)
      return noContent()
    },
  },
]
