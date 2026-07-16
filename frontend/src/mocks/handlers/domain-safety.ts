// Domain safety handlers: DKIM signing domains (CRUD + key generation),
// suppression list (CRUD), and Require-TLS policies (CRUD).

import { all, createRow, genId, paged, removeRow, updateRow } from '../db'
import { fingerprint } from '../fixtures/domain-safety'
import { noContent, notFound, ok, type Route } from '../router'

function pem(): string {
  return [
    '-----BEGIN PRIVATE KEY-----',
    'MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQ',
    'DZxF2N9qJ3k0pQxR7uLm1a8sT4vWcBnH5kPq2YxKjR8sM4nUv',
    '-----END PRIVATE KEY-----',
  ].join('\n')
}

export const domainSafetyRoutes: Route[] = [
  // ---- DKIM domains ----
  { method: 'GET', pattern: '/dkim-domains', handler: (ctx) => ok(paged(all('dkimDomains'), ctx.query)) },
  {
    method: 'POST',
    pattern: '/dkim-domains',
    handler: (ctx) => {
      const body = ctx.body as { domain: string; selector: string; public_key_fingerprint: string }
      return ok(createRow('dkimDomains', {
        id: genId('dkim'),
        domain: body.domain,
        selector: body.selector,
        publicKeyFingerprint: body.public_key_fingerprint || fingerprint(),
        status: 'needs_attention',
      }))
    },
  },
  {
    method: 'PUT',
    pattern: '/dkim-domains/:id',
    handler: (ctx) => {
      const body = ctx.body as { selector: string; public_key_fingerprint: string; status: string }
      const updated = updateRow('dkimDomains', ctx.params.id, { selector: body.selector, publicKeyFingerprint: body.public_key_fingerprint, status: body.status })
      return updated ? ok(updated) : notFound('DKIM domain not found')
    },
  },
  {
    method: 'POST',
    pattern: '/dkim-domains:generate-key',
    handler: (ctx) => {
      const body = ctx.body as { domain: string; selector: string }
      return ok({
        privateKeyPem: pem(),
        recordName: `${body.selector}._domainkey.${body.domain}`,
        recordValue: `v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA${fingerprint().replace(/:/g, '')}`,
        publicKeyFingerprint: fingerprint(),
      })
    },
  },

  // ---- Suppressions ----
  {
    method: 'GET',
    pattern: '/suppressions',
    handler: (ctx) => {
      // Mirrors the backend's ListSuppressions filters.
      const q = (name: string) => (ctx.query[name] || '').toString().toLowerCase()
      const search = q('search')
      const type = q('type')
      const status = q('status')
      const source = q('source')
      const expiry = q('expiry')
      const mailclass = (ctx.query.mailclass || '').toString()
      const rows = all('suppressions').filter((row) => {
        const s = row as {
          value: string
          type?: string
          status?: string
          source?: string
          mailclass?: string
          expiresAt?: string | null
        }
        if (search && !s.value.toLowerCase().includes(search)) return false
        if (type && (s.type ?? '').toLowerCase() !== type) return false
        if (status && (s.status ?? '').toLowerCase() !== status) return false
        if (source && (s.source ?? '').toLowerCase() !== source) return false
        if (expiry === 'permanent' && s.expiresAt) return false
        if (expiry === 'temporary' && !s.expiresAt) return false
        if (mailclass && !(s.mailclass ?? '').toLowerCase().includes(mailclass.toLowerCase())) return false
        return true
      })

      // Backend-driven sort: whitelist of columns, mapped to the row fields.
      const sortFields: Record<string, string> = {
        value: 'value',
        type: 'type',
        source: 'source',
        status: 'status',
        mailclass: 'mailclass',
        reason: 'reason',
        created_at: 'createdAt',
        expires_at: 'expiresAt',
      }
      const field = sortFields[q('sort')] ?? 'value'
      const desc = (ctx.query.desc || '').toString() === 'true'
      rows.sort((a, b) => {
        const av = ((a as unknown as Record<string, unknown>)[field] ?? '') as string
        const bv = ((b as unknown as Record<string, unknown>)[field] ?? '') as string
        // Empty values sort last regardless of direction (NULLS LAST parity).
        if (av === '' && bv !== '') return 1
        if (bv === '' && av !== '') return -1
        const cmp = String(av).localeCompare(String(bv))
        return desc ? -cmp : cmp
      })
      return ok(paged(rows, ctx.query))
    },
  },
  {
    method: 'POST',
    pattern: '/suppressions',
    handler: (ctx) => {
      const body = ctx.body as { type: 'email' | 'domain'; value: string; reason: string }
      return ok(createRow('suppressions', {
        id: genId('sup'),
        type: body.type,
        value: body.value,
        reason: body.reason,
        source: 'manual',
        status: 'active',
        createdAt: new Date().toISOString(),
      }))
    },
  },
  {
    method: 'PUT',
    pattern: '/suppressions/:id',
    handler: (ctx) => {
      const body = ctx.body as { reason: string; status: string }
      const updated = updateRow('suppressions', ctx.params.id, { reason: body.reason, status: body.status })
      return updated ? ok(updated) : notFound('Suppression not found')
    },
  },
  {
    method: 'GET',
    pattern: '/suppressions/:id/dsn-messages',
    handler: (ctx) => {
      const sup = all('suppressions').find((s) => (s as { id: string }).id === ctx.params.id) as
        | { value: string; source: string }
        | undefined
      if (!sup || sup.source !== 'dsn') return ok({ items: [] })
      return ok({
        items: [
          {
            id: `dsn_${ctx.params.id}`,
            messageId: 'a1b2c3d4e5f60718293a4b5c6d7e8f90',
            receivedAt: new Date().toISOString(),
            rawMessage: [
              'Return-Path: <>',
              'From: Mail Delivery System <MAILER-DAEMON@mx.example.com>',
              `To: <${sup.value}>`,
              'Subject: Undelivered Mail Returned to Sender',
              'Content-Type: multipart/report; report-type=delivery-status;',
              '',
              'This is the mail system at host mx.example.com.',
              '',
              'I\'m sorry to have to inform you that your message could not',
              'be delivered to one or more recipients.',
              '',
              '--- The following addresses had permanent fatal errors ---',
              `<${sup.value}>`,
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

  // ---- TLS policies ----
  {
    method: 'GET',
    pattern: '/tls-policies',
    handler: (ctx) => {
      const q = (ctx.query.search ?? '').toLowerCase()
      return ok(paged(all('tlsPolicies'), ctx.query, { filter: (p) => !q || p.domain.toLowerCase().includes(q) }))
    },
  },
  {
    method: 'POST',
    pattern: '/tls-policies',
    handler: (ctx) => {
      const body = ctx.body as { domain: string; mode: string }
      return ok(
        createRow('tlsPolicies', {
          id: genId('tls'),
          domain: body.domain,
          mode: body.mode,
          status: 'active',
          source: 'manual',
          createdAt: new Date().toISOString(),
        }),
      )
    },
  },
  {
    method: 'DELETE',
    pattern: '/tls-policies/:id',
    handler: (ctx) => (removeRow('tlsPolicies', ctx.params.id) ? noContent() : notFound('TLS policy not found')),
  },
]
