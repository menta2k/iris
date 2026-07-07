// Mock handlers for the Event Processor console.

import type { EventProcessor } from '../../types'
import { all, createRow, genId, removeRow, updateRow } from '../db'
import { noContent, notFound, ok, type Route } from '../router'

function fromBody(body: Record<string, unknown>): Omit<EventProcessor, 'id' | 'status'> {
  return {
    name: String(body.name ?? ''),
    eventTypes: (body.event_types as string[]) ?? [],
    mailclasses: (body.mailclasses as string[]) ?? [],
    driver: String(body.driver ?? 'webhook'),
    driverConfig: (body.driver_config as Record<string, string>) ?? {},
    mode: String(body.mode ?? 'single'),
    batchMaxSize: Number(body.batch_max_size ?? 0),
    batchMaxWait: String(body.batch_max_wait ?? ''),
  }
}

export const eventProcessorsRoutes: Route[] = [
  { method: 'GET', pattern: '/event-processors', handler: () => ok({ items: all('eventProcessors') }) },
  {
    method: 'POST',
    pattern: '/event-processors',
    handler: (ctx) =>
      ok(createRow('eventProcessors', { id: genId('ep'), status: 'active', ...fromBody(ctx.body as Record<string, unknown>) } as EventProcessor)),
  },
  {
    method: 'PUT',
    pattern: '/event-processors/:id',
    handler: (ctx) => {
      const body = ctx.body as Record<string, unknown>
      const updated = updateRow('eventProcessors', ctx.params.id, {
        ...fromBody(body),
        status: (body.status as EventProcessor['status']) || 'active',
      })
      return updated ? ok(updated) : notFound('Event processor not found')
    },
  },
  {
    method: 'DELETE',
    pattern: '/event-processors/:id',
    handler: (ctx) => (removeRow('eventProcessors', ctx.params.id) ? noContent() : notFound('Event processor not found')),
  },
  {
    method: 'POST',
    pattern: '/event-processors:test',
    handler: (ctx) => {
      const body = ctx.body as Record<string, unknown>
      const cfg = (body.driver_config as Record<string, string>) ?? {}
      // Mimic a driver validation: webhook needs a url, redis needs a stream.
      const okDriver = body.driver === 'webhook' ? !!cfg.url : !!cfg.stream
      return ok(okDriver ? { ok: true } : { ok: false, error: 'driver not configured (missing url/stream)' })
    },
  },
]
