// Inbox-placement monitoring handlers: mailbox account CRUD, probe send + list.
// Passwords are write-only (never echoed); the dev mock tracks hasPassword only.

import { all, createRow, genId, removeRow, updateRow } from '../db'
import { notFound, ok, type Route } from '../router'
import { minutesAgo } from '../fixtures/util'
import type { MonitoringAccount, MonitoringProbe } from '../../types'

interface AccountBody {
  label?: string
  provider?: MonitoringAccount['provider']
  email?: string
  protocol?: MonitoringAccount['protocol']
  host?: string
  port?: number
  tls?: boolean
  username?: string
  password?: string
  checkFolders?: string[]
  fromAddress?: string
  scheduleEnabled?: boolean
  scheduleInterval?: string
  fetchDelay?: string
  enabled?: boolean
}

function accountsFor(accountId: string): MonitoringProbe[] {
  return (all('monitoringProbes') as MonitoringProbe[]).filter((p) => p.accountId === accountId)
}

function probeUid(): string {
  return 'ip' + genId('').replace(/[^a-z0-9]/gi, '').slice(0, 24)
}

export const monitoringRoutes: Route[] = [
  { method: 'GET', pattern: '/monitoring/accounts', handler: () => ok({ items: all('monitoringAccounts') }) },
  {
    method: 'POST',
    pattern: '/monitoring/accounts',
    handler: (ctx) => {
      const b = ctx.body as AccountBody
      return ok(
        createRow('monitoringAccounts', {
          id: genId('mon'),
          label: b.label ?? '',
          provider: b.provider ?? 'custom',
          email: b.email ?? '',
          protocol: b.protocol ?? 'imap',
          host: b.host ?? '',
          port: b.port ?? 993,
          tls: b.tls ?? true,
          username: b.username || (b.email ?? ''),
          checkFolders: b.checkFolders ?? ['INBOX'],
          fromAddress: b.fromAddress ?? '',
          scheduleEnabled: b.scheduleEnabled ?? false,
          scheduleInterval: b.scheduleInterval ?? '',
          fetchDelay: b.fetchDelay ?? '10m',
          enabled: b.enabled ?? true,
          hasPassword: !!b.password,
          createdAt: minutesAgo(0),
          updatedAt: minutesAgo(0),
        }),
      )
    },
  },
  {
    method: 'PUT',
    pattern: '/monitoring/accounts/:id',
    handler: (ctx) => {
      const b = ctx.body as AccountBody
      const updated = updateRow('monitoringAccounts', ctx.params.id, {
        label: b.label ?? '',
        provider: b.provider ?? 'custom',
        email: b.email ?? '',
        protocol: b.protocol ?? 'imap',
        host: b.host ?? '',
        port: b.port ?? 993,
        tls: b.tls ?? true,
        username: b.username ?? '',
        checkFolders: b.checkFolders ?? ['INBOX'],
        fromAddress: b.fromAddress ?? '',
        scheduleEnabled: b.scheduleEnabled ?? false,
        scheduleInterval: b.scheduleInterval ?? '',
        fetchDelay: b.fetchDelay ?? '10m',
        enabled: b.enabled ?? true,
        updatedAt: minutesAgo(0),
      })
      return updated ? ok(updated) : notFound('Monitoring account not found')
    },
  },
  {
    method: 'POST',
    pattern: '/monitoring/accounts/:id/password',
    handler: (ctx) => {
      const updated = updateRow('monitoringAccounts', ctx.params.id, {
        hasPassword: true,
        updatedAt: minutesAgo(0),
      })
      return updated ? ok(updated) : notFound('Monitoring account not found')
    },
  },
  {
    method: 'DELETE',
    pattern: '/monitoring/accounts/:id',
    handler: (ctx) =>
      removeRow('monitoringAccounts', ctx.params.id) ? ok({ ok: true }) : notFound('Monitoring account not found'),
  },
  {
    method: 'POST',
    pattern: '/monitoring/accounts:verify',
    handler: (ctx) => {
      const b = ctx.body as { host?: string; password?: string; id?: string }
      // Dev mock: succeed when a host is present and a password is supplied or an
      // existing account is referenced.
      if (b.host && (b.password || b.id)) return ok({ ok: true })
      return ok({ ok: false, error: 'Mock: missing host or password.' })
    },
  },
  {
    method: 'POST',
    pattern: '/monitoring/accounts/:id/probe',
    handler: (ctx) => {
      const account = (all('monitoringAccounts') as MonitoringAccount[]).find((a) => a.id === ctx.params.id)
      if (!account) return notFound('Monitoring account not found')
      const uid = probeUid()
      return ok(
        createRow('monitoringProbes', {
          id: genId('prb'),
          accountId: account.id,
          probeUid: uid,
          messageId: '',
          subject: `[iris-probe] ${uid}`,
          fromAddr: `probe+${uid}@monitor.example.com`,
          recipient: account.email,
          sentAt: minutesAgo(0),
          sendStatus: 'queued',
          mailboxStatus: 'pending',
          placement: '',
          latencyMs: 0,
          analysis: '{}',
          error: '',
          createdAt: minutesAgo(0),
          updatedAt: minutesAgo(0),
        }),
      )
    },
  },
  {
    method: 'GET',
    pattern: '/monitoring/probes/:id/events',
    handler: (ctx) => {
      const p = (all('monitoringProbes') as MonitoringProbe[]).find((x) => x.id === ctx.params.id)
      if (!p) return ok({ items: [] })
      const items: Array<{ id: string; at: string; phase: string; level: string; message: string }> = [
        { id: 'e1', at: p.sentAt ?? minutesAgo(20), phase: 'send', level: 'info', message: `Injected into KumoMTA from ${p.fromAddr} to ${p.recipient}; queued.` },
      ]
      if (p.sendStatus === 'sent') items.push({ id: 'e2', at: p.sentAt ?? minutesAgo(19), phase: 'send', level: 'info', message: 'Delivery confirmed by KumoMTA: sent.' })
      if (p.mailboxStatus === 'found') {
        items.push({ id: 'e3', at: p.foundAt ?? minutesAgo(9), phase: 'fetch', level: 'info', message: `Found in ${p.placement === 'spam' ? '[Gmail]/Spam' : 'INBOX'} → placement: ${p.placement}.` })
        items.push({ id: 'e4', at: p.foundAt ?? minutesAgo(9), phase: 'analyze', level: 'info', message: 'Header analysis complete → spam risk: ' + (p.placement === 'spam' ? 'spam' : 'clean') + '.' })
      }
      return ok({ items })
    },
  },
  {
    method: 'GET',
    pattern: '/monitoring/accounts/:id/probes',
    handler: (ctx) => {
      const items = accountsFor(ctx.params.id).slice().sort((a, b) => (a.sentAt! < b.sentAt! ? 1 : -1))
      return ok({ items })
    },
  },
  {
    method: 'GET',
    pattern: '/monitoring/probes/:id/raw',
    handler: (ctx) => {
      const p = (all('monitoringProbes') as MonitoringProbe[]).find((x) => x.id === ctx.params.id)
      if (!p) return notFound('Monitoring probe not found')
      const headers = `From: iris monitor <${p.fromAddr}>\r\nTo: ${p.recipient}\r\nSubject: ${p.subject}\r\n${ProbeUidHeader}: ${p.probeUid}`
      return ok({
        id: p.id,
        probeUid: p.probeUid,
        subject: p.subject,
        recipient: p.recipient,
        rawHeaders: headers,
        rawMessage: `${headers}\r\n\r\nThis is an automated iris inbox-placement probe.\r\nProbe ID: ${p.probeUid}\r\n`,
      })
    },
  },
]

const ProbeUidHeader = 'X-Iris-Probe-Id'
