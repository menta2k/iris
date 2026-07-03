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
  { method: 'GET', pattern: '/suppressions', handler: (ctx) => ok(paged(all('suppressions'), ctx.query)) },
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

  // ---- TLS policies ----
  { method: 'GET', pattern: '/tls-policies', handler: (ctx) => ok(paged(all('tlsPolicies'), ctx.query)) },
  {
    method: 'POST',
    pattern: '/tls-policies',
    handler: (ctx) => {
      const body = ctx.body as { domain: string; mode: string }
      return ok(createRow('tlsPolicies', { id: genId('tls'), domain: body.domain, mode: body.mode, status: 'active' }))
    },
  },
  {
    method: 'DELETE',
    pattern: '/tls-policies/:id',
    handler: (ctx) => (removeRow('tlsPolicies', ctx.params.id) ? noContent() : notFound('TLS policy not found')),
  },
]
