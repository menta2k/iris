// Settings handlers: deployment-level global settings (singleton), retention
// views + policy updates, and subject-line classifications (CRUD).

import type { GlobalSettings, RetentionPolicy, UpdateRetentionPolicyRequest } from '../../types'
import { all, createRow, genId, removeRow, updateRow } from '../db'
import { globalSettings, retentionViews } from '../fixtures/settings'
import { daysAgo, hoursAgo } from '../fixtures/util'
import { notFound, ok, type Route } from '../router'

let settings: GlobalSettings = { ...globalSettings }
let views = retentionViews.slice()

function snakeToCamel(key: string): string {
  return key.replace(/_([a-z])/g, (_, c: string) => c.toUpperCase())
}

function mapRetentionPolicy(body: UpdateRetentionPolicyRequest): RetentionPolicy {
  return {
    tableName: body.table_name,
    retentionDays: body.retention_days,
    compressAfterDays: body.compress_after_days,
    enabled: body.enabled,
    updatedAt: hoursAgo(0),
    updatedBy: 'admin@iris.local',
  }
}

export const settingsRoutes: Route[] = [
  // ---- Global settings ----
  { method: 'GET', pattern: '/settings', handler: () => ok(settings) },
  {
    method: 'PUT',
    pattern: '/settings',
    handler: (ctx) => {
      const next: Record<string, unknown> = { ...settings }
      for (const [key, value] of Object.entries(ctx.body as Record<string, unknown>)) {
        next[snakeToCamel(key)] = value
      }
      next.updatedAt = hoursAgo(0)
      next.updatedBy = 'admin@iris.local'
      settings = next as unknown as GlobalSettings
      return ok(settings)
    },
  },

  // ---- Retention ----
  { method: 'GET', pattern: '/retention', handler: () => ok({ items: views }) },
  {
    method: 'PUT',
    pattern: '/retention/:table',
    handler: (ctx) => {
      const policy = mapRetentionPolicy(ctx.body as UpdateRetentionPolicyRequest)
      const idx = views.findIndex((v) => v.policy.tableName === ctx.params.table)
      if (idx === -1) return notFound('Retention policy not found')
      views = views.map((v, i) => (i === idx ? { ...v, policy } : v))
      return ok(policy)
    },
  },
  {
    method: 'POST',
    pattern: '/retention:run',
    handler: () => ok({ ok: true }),
  },

  // ---- Subject classifications ----
  {
    method: 'GET',
    pattern: '/subject-classifications',
    // Mirror the backend List ordering: highest priority first, then most-used.
    handler: () =>
      ok({
        items: [...all('classifications')].sort(
          (a, b) => b.priority - a.priority || Number(b.hitCount) - Number(a.hitCount),
        ),
      }),
  },
  {
    method: 'POST',
    pattern: '/subject-classifications',
    handler: (ctx) => {
      const body = ctx.body as { subject: string; label: string; matchType?: string; priority?: number }
      const matchType = body.matchType === 'regex' ? 'regex' : 'similarity'
      return ok(createRow('classifications', {
        id: genId('cls'),
        subject: body.subject,
        subjectNormalized: matchType === 'regex' ? '' : body.subject.toLowerCase(),
        label: body.label,
        source: 'manual',
        matchType,
        priority: Number(body.priority) || 0,
        hitCount: '0',
        createdAt: daysAgo(0),
        updatedAt: daysAgo(0),
      }))
    },
  },
  {
    method: 'PUT',
    pattern: '/subject-classifications/:id',
    handler: (ctx) => {
      const body = ctx.body as { id: string; subject: string; label: string; matchType?: string; priority?: number }
      const matchType = body.matchType === 'regex' ? 'regex' : 'similarity'
      const updated = updateRow('classifications', ctx.params.id, {
        subject: body.subject,
        subjectNormalized: matchType === 'regex' ? '' : body.subject.toLowerCase(),
        label: body.label,
        matchType,
        priority: Number(body.priority) || 0,
        updatedAt: daysAgo(0),
      })
      return updated ? ok(updated) : notFound('Classification not found')
    },
  },
  {
    method: 'DELETE',
    pattern: '/subject-classifications/:id',
    handler: (ctx) => (removeRow('classifications', ctx.params.id) ? ok({ ok: true }) : notFound('Classification not found')),
  },
]
