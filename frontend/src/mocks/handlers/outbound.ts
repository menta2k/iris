// Outbound config handlers: listeners, VMTAs, VMTA groups, routing rules, IP
// warmup schedules, delivery blueprints, and TSA automation rules. CRUD mutates
// the in-memory DB so the UI reflects creates/edits/deletes live.

import type {
  CreateListenerRequest,
  CreateRoutingRuleRequest,
  CreateVMTAGroupRequest,
  CreateVMTARequest,
  Listener,
  UpdateListenerRequest,
  UpdateRoutingRuleRequest,
  UpdateVMTAGroupRequest,
  UpdateVMTARequest,
  VMTA,
} from '../../types'
import { all, createRow, findRow, genId, paged, updateRow } from '../db'
import { notFound, ok, type Route } from '../router'

function list<T>(rows: T[], ctx: { query: Record<string, string> }, filter?: (row: T) => boolean): unknown {
  return paged(rows, ctx.query, { filter })
}

function listenerFromCreate(body: CreateListenerRequest): Listener {
  return {
    id: genId('lst'),
    name: body.name,
    ipAddress: body.ip_address,
    port: body.port,
    hostname: body.hostname,
    tlsEnabled: body.tls_enabled,
    tlsCertPath: body.tls_cert_path,
    tlsKeyPath: body.tls_key_path,
    requireAuth: body.require_auth,
    maxMessageSize: body.max_message_size,
    relayHosts: body.relay_hosts ?? [],
    status: 'active',
    role: body.role,
    nodeId: body.node_id ?? '',
    nodeName: (body.node_id ? findRow('mtaNodes', body.node_id)?.name : '') ?? '',
  }
}

function listenerFromUpdate(existing: Listener, body: UpdateListenerRequest): Listener {
  return {
    ...listenerFromCreate(body),
    id: existing.id,
    status: body.status || existing.status,
  }
}

function vmtaFromCreate(body: CreateVMTARequest): VMTA {
  const listener = body.listener_id ? findRow('listeners', body.listener_id) : undefined
  return {
    id: genId('vmta'),
    name: body.name,
    status: 'ACTIVE',
    notes: '',
    listenerId: body.listener_id ?? '',
    listenerName: listener?.name ?? '',
    ipAddress: body.ip_address,
    ehloName: body.ehlo_name,
    maxConnections: body.max_connections,
    tlsMode: body.tls_mode ?? '',
    nodeId: body.node_id ?? '',
    nodeName: (body.node_id ? findRow('mtaNodes', body.node_id)?.name : '') ?? '',
  }
}

export const outboundRoutes: Route[] = [
  // ---- Listeners ----
  {
    method: 'GET',
    pattern: '/listeners',
    handler: (ctx) =>
      ok(list(all('listeners'), ctx, ctx.query.status ? (l) => l.status === ctx.query.status : undefined)),
  },
  { method: 'POST', pattern: '/listeners', handler: (ctx) => ok(createRow('listeners', listenerFromCreate(ctx.body as CreateListenerRequest))) },
  {
    method: 'PUT',
    pattern: '/listeners/:id',
    handler: (ctx) => {
      const existing = findRow('listeners', ctx.params.id)
      return existing ? ok(updateRow('listeners', ctx.params.id, listenerFromUpdate(existing, ctx.body as UpdateListenerRequest))) : notFound('Listener not found')
    },
  },

  // ---- VMTAs ----
  {
    method: 'GET',
    pattern: '/vmtas',
    handler: (ctx) => ok(list(all('vmtas'), ctx, ctx.query.status ? (v) => v.status === ctx.query.status : undefined)),
  },
  { method: 'POST', pattern: '/vmtas', handler: (ctx) => ok(createRow('vmtas', vmtaFromCreate(ctx.body as CreateVMTARequest))) },
  {
    method: 'PUT',
    pattern: '/vmtas/:id',
    handler: (ctx) => {
      const body = ctx.body as UpdateVMTARequest
      const listener = body.listener_id ? findRow('listeners', body.listener_id) : undefined
      const patch: Partial<VMTA> = {
        name: body.name,
        ipAddress: body.ip_address,
        ehloName: body.ehlo_name,
        listenerId: body.listener_id ?? '',
        listenerName: listener?.name ?? '',
        maxConnections: body.max_connections,
        status: body.status,
        notes: body.notes,
        tlsMode: body.tls_mode ?? '',
        nodeId: body.node_id ?? '',
        nodeName: (body.node_id ? findRow('mtaNodes', body.node_id)?.name : '') ?? '',
      }
      const updated = updateRow('vmtas', ctx.params.id, patch)
      return updated ? ok(updated) : notFound('VMTA not found')
    },
  },

  // ---- VMTA groups ----
  { method: 'GET', pattern: '/vmta-groups', handler: (ctx) => ok(list(all('vmtaGroups'), ctx)) },
  {
    method: 'POST',
    pattern: '/vmta-groups',
    handler: (ctx) => {
      const body = ctx.body as CreateVMTAGroupRequest
      const group = {
        id: genId('grp'),
        name: body.name,
        status: 'ACTIVE',
        members: (body.members ?? []).map((m) => ({ vmtaId: m.vmta_id, weight: m.weight })),
      }
      return ok(createRow('vmtaGroups', group))
    },
  },
  {
    method: 'PUT',
    pattern: '/vmta-groups/:id',
    handler: (ctx) => {
      const body = ctx.body as UpdateVMTAGroupRequest
      const patch = {
        name: body.name,
        status: body.status,
        members: (body.members ?? []).map((m) => ({ vmtaId: m.vmta_id, weight: m.weight })),
      }
      const updated = updateRow('vmtaGroups', ctx.params.id, patch)
      return updated ? ok(updated) : notFound('VMTA group not found')
    },
  },

  // ---- Routing rules ----
  {
    method: 'GET',
    pattern: '/routing-rules',
    handler: (ctx) => {
      const mt = ctx.query.match_type
      const mv = ctx.query.match_value
      const filter =
        mt || mv
          ? (r: { matchType: string; matchValue: string }) =>
              (!mt || r.matchType === mt) && (!mv || r.matchValue.includes(mv))
          : undefined
      return ok(list(all('routingRules'), ctx, filter))
    },
  },
  {
    method: 'POST',
    pattern: '/routing-rules',
    handler: (ctx) => {
      const body = ctx.body as CreateRoutingRuleRequest
      const rule = {
        id: genId('rule'),
        name: body.name,
        matchType: body.match_type,
        matchHeader: body.match_header,
        matchValue: body.match_value,
        conditions: body.conditions ?? [],
        priority: body.priority,
        targetType: body.target_type,
        targetId: body.target_id,
        assignMailclass: body.assign_mailclass,
        status: 'active',
      }
      return ok(createRow('routingRules', rule))
    },
  },
  {
    method: 'PUT',
    pattern: '/routing-rules/:id',
    handler: (ctx) => {
      const body = ctx.body as UpdateRoutingRuleRequest
      const patch = {
        name: body.name,
        matchType: body.match_type,
        matchHeader: body.match_header,
        matchValue: body.match_value,
        conditions: body.conditions ?? [],
        priority: body.priority,
        targetType: body.target_type,
        targetId: body.target_id,
        assignMailclass: body.assign_mailclass,
        status: body.status,
      }
      const updated = updateRow('routingRules', ctx.params.id, patch)
      return updated ? ok(updated) : notFound('Routing rule not found')
    },
  },

  // ---- IP warmup ----
  {
    method: 'GET',
    pattern: '/warmup-schedules',
    handler: (ctx) => {
      const rows = ctx.query.status ? all('warmupSchedules').filter((s) => s.status === ctx.query.status) : all('warmupSchedules')
      return ok({ items: rows, curves: WARMUP_CURVES })
    },
  },
  {
    method: 'POST',
    pattern: '/warmup-schedules',
    handler: (ctx) => {
      const body = ctx.body as { vmta_id: string; start_date: string; curve: string }
      const vmta = findRow('vmtas', body.vmta_id)
      const schedule = {
        id: genId('wrm'),
        vmtaId: body.vmta_id,
        vmtaName: vmta?.name ?? body.vmta_id,
        startDate: body.start_date,
        curve: body.curve,
        stages: WARMUP_CURVES.find((c) => c.name === body.curve)?.stages ?? [],
        status: 'scheduled' as const,
      }
      return ok(createRow('warmupSchedules', schedule))
    },
  },
  {
    method: 'PUT',
    pattern: '/warmup-schedules/:id',
    handler: (ctx) => {
      const body = ctx.body as { start_date: string; curve: string; stages?: Array<{ day_from: number; day_to: number; caps: Record<string, number> }> }
      const patch = {
        startDate: body.start_date,
        curve: body.curve,
        stages: body.stages?.map((s) => ({ dayFrom: s.day_from, dayTo: s.day_to, caps: s.caps })) ?? [],
      }
      const updated = updateRow('warmupSchedules', ctx.params.id, patch)
      return updated ? ok(updated) : notFound('Warmup schedule not found')
    },
  },
  {
    method: 'POST',
    pattern: '/warmup-schedules/:id:pause',
    handler: (ctx) => {
      const body = ctx.body as { reason: string }
      const updated = updateRow('warmupSchedules', ctx.params.id, { status: 'paused', pausedReason: body.reason })
      return updated ? ok(updated) : notFound('Warmup schedule not found')
    },
  },
  {
    method: 'POST',
    pattern: '/warmup-schedules/:id:resume',
    handler: (ctx) => {
      const updated = updateRow('warmupSchedules', ctx.params.id, { status: 'active', pausedReason: undefined })
      return updated ? ok(updated) : notFound('Warmup schedule not found')
    },
  },

  // ---- Delivery blueprints ----
  { method: 'GET', pattern: '/delivery-blueprints', handler: (ctx) => ok(list(all('blueprints'), ctx)) },
  {
    method: 'POST',
    pattern: '/delivery-blueprints',
    handler: (ctx) => {
      const body = ctx.body as { provider: string; mx_pattern: string; conn_rate: string; deliveries_per_conn: number; conn_limit: number; daily_cap: number }
      return ok(createRow('blueprints', { id: genId('bp'), provider: body.provider, mxPattern: body.mx_pattern, connRate: body.conn_rate, deliveriesPerConn: body.deliveries_per_conn, connLimit: body.conn_limit, dailyCap: body.daily_cap, status: 'active' }))
    },
  },
  {
    method: 'PUT',
    pattern: '/delivery-blueprints/:id',
    handler: (ctx) => {
      const body = ctx.body as { provider: string; mx_pattern: string; conn_rate: string; deliveries_per_conn: number; conn_limit: number; daily_cap: number; status: 'active' | 'disabled' }
      const updated = updateRow('blueprints', ctx.params.id, { provider: body.provider, mxPattern: body.mx_pattern, connRate: body.conn_rate, deliveriesPerConn: body.deliveries_per_conn, connLimit: body.conn_limit, dailyCap: body.daily_cap, status: body.status })
      return updated ? ok(updated) : notFound('Blueprint not found')
    },
  },
  {
    method: 'POST',
    pattern: '/delivery-blueprints/:id:status',
    handler: (ctx) => {
      const body = ctx.body as { status: 'active' | 'disabled' }
      const updated = updateRow('blueprints', ctx.params.id, { status: body.status })
      return updated ? ok(updated) : notFound('Blueprint not found')
    },
  },
  {
    method: 'POST',
    pattern: '/delivery-blueprints:seed-defaults',
    handler: () => ok({ inserted: 4 }),
  },

  // ---- TSA automation rules ----
  { method: 'GET', pattern: '/automation-rules', handler: (ctx) => ok(list(all('automationRules'), ctx)) },
  {
    method: 'POST',
    pattern: '/automation-rules',
    handler: (ctx) => {
      const body = ctx.body as { domain: string; regex: string; action: string; config_name: string; config_value: string; trigger: string; duration: string }
      return ok(createRow('automationRules', { id: genId('auto'), domain: body.domain, regex: body.regex, action: body.action as never, configName: body.config_name, configValue: body.config_value, trigger: body.trigger, duration: body.duration, status: 'active' }))
    },
  },
  {
    method: 'PUT',
    pattern: '/automation-rules/:id',
    handler: (ctx) => {
      const body = ctx.body as { domain: string; regex: string; action: string; config_name: string; config_value: string; trigger: string; duration: string; status: 'active' | 'disabled' }
      const updated = updateRow('automationRules', ctx.params.id, { domain: body.domain, regex: body.regex, action: body.action as never, configName: body.config_name, configValue: body.config_value, trigger: body.trigger, duration: body.duration, status: body.status })
      return updated ? ok(updated) : notFound('Automation rule not found')
    },
  },
  {
    method: 'POST',
    pattern: '/automation-rules/:id:status',
    handler: (ctx) => {
      const body = ctx.body as { status: 'active' | 'disabled' }
      const updated = updateRow('automationRules', ctx.params.id, { status: body.status })
      return updated ? ok(updated) : notFound('Automation rule not found')
    },
  },
]

const WARMUP_CURVES = [
  { name: 'standard-30', stages: [{ dayFrom: 1, dayTo: 7, caps: { gmail: 1000, yahoo: 400 } }, { dayFrom: 8, dayTo: 30, caps: { gmail: 20000, yahoo: 8000 } }] },
  { name: 'aggressive-14', stages: [{ dayFrom: 1, dayTo: 14, caps: { gmail: 10000, yahoo: 4000 } }] },
  { name: 'conservative-45', stages: [{ dayFrom: 1, dayTo: 45, caps: { gmail: 800, yahoo: 300 } }] },
]
