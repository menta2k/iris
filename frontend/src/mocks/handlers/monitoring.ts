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
    pattern: '/monitoring/accounts/:id/probes',
    handler: (ctx) => {
      const items = accountsFor(ctx.params.id).slice().sort((a, b) => (a.sentAt! < b.sentAt! ? 1 : -1))
      return ok({ items })
    },
  },
]
