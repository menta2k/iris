// Security handlers: users (CRUD + admin password reset) and audit entries.
// The shared /mfa:* endpoints are handled in auth.ts.

import { all, createRow, genId, paged, removeRow, updateRow } from '../db'
import { noContent, notFound, ok, type Route } from '../router'
import { daysAgo } from '../fixtures/util'

export const securityRoutes: Route[] = [
  { method: 'GET', pattern: '/users', handler: (ctx) => ok(paged(all('users'), ctx.query)) },
  {
    method: 'POST',
    pattern: '/users',
    handler: (ctx) => {
      const body = ctx.body as { email: string; display_name: string; mfa_required: boolean; roles: string[] }
      return ok(createRow('users', {
        id: genId('usr'),
        email: body.email,
        displayName: body.display_name,
        status: 'active',
        mfaRequired: body.mfa_required,
        roles: body.roles,
      }))
    },
  },
  {
    method: 'PUT',
    pattern: '/users/:id',
    handler: (ctx) => {
      const body = ctx.body as { display_name: string; status: string; mfa_required: boolean; roles: string[] }
      const updated = updateRow('users', ctx.params.id, {
        displayName: body.display_name,
        status: body.status,
        mfaRequired: body.mfa_required,
        roles: body.roles,
      })
      return updated ? ok(updated) : notFound('User not found')
    },
  },
  {
    method: 'POST',
    pattern: '/users/:id:reset-password',
    handler: () => noContent(),
  },

  { method: 'GET', pattern: '/audit-entries', handler: (ctx) => ok(paged(all('auditEntries'), ctx.query, { defaultSize: 25 })) },

  // ---- Injection API credentials ----
  { method: 'GET', pattern: '/injection-credentials', handler: () => ok({ items: all('injectionCredentials') }) },
  {
    method: 'POST',
    pattern: '/injection-credentials',
    handler: (ctx) => {
      const body = ctx.body as {
        username: string
        label: string
        enabled: boolean
        allowedMailclasses?: string[]
      }
      return ok(createRow('injectionCredentials', {
        id: genId('inj'),
        username: body.username,
        label: body.label ?? '',
        enabled: body.enabled ?? true,
        allowedMailclasses: body.allowedMailclasses ?? [],
        createdAt: daysAgo(0),
        updatedAt: daysAgo(0),
      }))
    },
  },
  {
    method: 'PUT',
    pattern: '/injection-credentials/:id',
    handler: (ctx) => {
      const body = ctx.body as { label: string; enabled: boolean; allowedMailclasses?: string[] }
      const updated = updateRow('injectionCredentials', ctx.params.id, {
        label: body.label ?? '',
        enabled: body.enabled ?? true,
        allowedMailclasses: body.allowedMailclasses ?? [],
        updatedAt: daysAgo(0),
      })
      return updated ? ok(updated) : notFound('Injection credential not found')
    },
  },
  {
    method: 'POST',
    pattern: '/injection-credentials/:id/password',
    handler: (ctx) => {
      const updated = updateRow('injectionCredentials', ctx.params.id, { updatedAt: daysAgo(0) })
      return updated ? ok(updated) : notFound('Injection credential not found')
    },
  },
  {
    method: 'DELETE',
    pattern: '/injection-credentials/:id',
    handler: (ctx) => (removeRow('injectionCredentials', ctx.params.id) ? ok({ ok: true }) : notFound('Injection credential not found')),
  },
]
