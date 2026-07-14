// Cluster node registry handlers (/cluster/nodes CRUD).

import { all, createRow, genId, removeRow, updateRow } from '../db'
import { notFound, ok, type Route } from '../router'
import type { CreateMTANodeRequest, UpdateMTANodeRequest } from '../../types'

export const clusterRoutes: Route[] = [
  { method: 'GET', pattern: '/cluster/nodes', handler: () => ok({ items: all('mtaNodes') }) },
  {
    method: 'GET',
    pattern: '/cluster/nodes/:id',
    handler: (ctx) => {
      const node = all('mtaNodes').find((n) => n.id === ctx.params.id)
      return node ? ok(node) : notFound('Node not found')
    },
  },
  {
    method: 'POST',
    pattern: '/cluster/nodes',
    handler: (ctx) => {
      const body = ctx.body as CreateMTANodeRequest
      return ok(createRow('mtaNodes', {
        id: genId('node'),
        name: body.name,
        agentUrl: body.agentUrl ?? '',
        proxyHost: body.proxyHost ?? '',
        proxyPort: body.proxyPort ?? 0,
        status: body.status || 'active',
        certFingerprint: '',
        kumoState: '',
        version: '',
        appliedChecksum: '',
        notes: body.notes ?? '',
      }))
    },
  },
  {
    method: 'PUT',
    pattern: '/cluster/nodes/:id',
    handler: (ctx) => {
      const body = ctx.body as UpdateMTANodeRequest
      const updated = updateRow('mtaNodes', ctx.params.id, {
        name: body.name,
        agentUrl: body.agentUrl ?? '',
        proxyHost: body.proxyHost ?? '',
        proxyPort: body.proxyPort ?? 0,
        status: body.status,
        notes: body.notes ?? '',
      })
      return updated ? ok(updated) : notFound('Node not found')
    },
  },
  {
    method: 'DELETE',
    pattern: '/cluster/nodes/:id',
    handler: (ctx) => (removeRow('mtaNodes', ctx.params.id) ? ok({ ok: true }) : notFound('Node not found')),
  },
]
