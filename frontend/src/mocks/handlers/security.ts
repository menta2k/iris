// Security handlers: users (CRUD + admin password reset) and audit entries.
// The shared /mfa:* endpoints are handled in auth.ts.

import { all, createRow, genId, paged, updateRow } from '../db'
import { noContent, notFound, ok, type Route } from '../router'

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
]
