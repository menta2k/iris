// Mock handlers for the Bounce Based Actions console. The test-diagnostic route
// reimplements the backend matcher (priority, then specificity) so the mock
// behaves like production.

import type { BounceRule, TestBounceDiagnosticResult } from '../../types'
import { bounceRules as defaultBounceRules } from '../fixtures/bounce-rules'
import { all, createRow, genId, removeRow, updateRow } from '../db'
import { noContent, notFound, ok, type Route } from '../router'

const PROVIDER_SUFFIXES: Array<[string, string]> = [
  ['gmail.com', 'gmail'], ['googlemail.com', 'gmail'], ['google.com', 'gmail'],
  ['yahoo.com', 'yahoo'], ['ymail.com', 'yahoo'], ['aol.com', 'yahoo'],
  ['outlook.com', 'microsoft'], ['hotmail.com', 'microsoft'], ['live.com', 'microsoft'], ['msn.com', 'microsoft'],
  ['icloud.com', 'apple'], ['me.com', 'apple'],
]

function providerForDomain(domain: string): string {
  const d = domain.trim().toLowerCase()
  for (const [suffix, provider] of PROVIDER_SUFFIXES) {
    if (d === suffix || d.endsWith(`.${suffix}`)) return provider
  }
  return ''
}

function parseEnhanced(diagnostic: string): string {
  const m = diagnostic.match(/\b[245]\.\d{1,3}\.\d{1,3}\b/)
  return m ? m[0] : ''
}

function specificity(r: BounceRule): number {
  return [r.smtpCode, r.enhancedCode, r.provider, r.pattern].filter(Boolean).length
}

function matches(r: BounceRule, sig: { smtpCode: string; enhancedCode: string; provider: string; diagnostic: string }): boolean {
  if (r.status !== 'active') return false
  if (r.smtpCode && r.smtpCode !== sig.smtpCode) return false
  if (r.enhancedCode && r.enhancedCode !== sig.enhancedCode) return false
  if (r.provider && r.provider !== sig.provider) return false
  if (r.pattern && !sig.diagnostic.toLowerCase().includes(r.pattern.toLowerCase())) return false
  return true
}

function toUpdate(body: Record<string, unknown>): Partial<BounceRule> {
  return {
    smtpCode: String(body.smtp_code ?? ''),
    enhancedCode: String(body.enhanced_code ?? ''),
    provider: String(body.provider ?? ''),
    pattern: String(body.pattern ?? ''),
    class: (body.class as BounceRule['class']) || 'soft',
    category: String(body.category ?? ''),
    action: body.action as BounceRule['action'],
    actionConfig: String(body.action_config ?? ''),
    suggestedAction: String(body.suggested_action ?? ''),
    priority: Number(body.priority ?? 0),
  }
}

export const bounceRulesRoutes: Route[] = [
  { method: 'GET', pattern: '/bounce-rules', handler: () => ok({ items: all('bounceRules') }) },
  {
    method: 'POST',
    pattern: '/bounce-rules',
    handler: (ctx) => {
      const body = ctx.body as Record<string, unknown>
      return ok(createRow('bounceRules', { id: genId('br'), source: 'overlay', status: 'active', ...toUpdate(body) } as BounceRule))
    },
  },
  {
    method: 'PUT',
    pattern: '/bounce-rules/:id',
    handler: (ctx) => {
      const body = ctx.body as Record<string, unknown>
      const patch = { ...toUpdate(body), status: (body.status as BounceRule['status']) || 'active' }
      const updated = updateRow('bounceRules', ctx.params.id, patch)
      return updated ? ok(updated) : notFound('Bounce rule not found')
    },
  },
  {
    method: 'DELETE',
    pattern: '/bounce-rules/:id',
    handler: (ctx) => (removeRow('bounceRules', ctx.params.id) ? noContent() : notFound('Bounce rule not found')),
  },
  {
    method: 'POST',
    pattern: '/bounce-rules:reset',
    handler: () => {
      // Keep overlay rules; replace the default-source set with the seeded defaults.
      const overlay = all('bounceRules').filter((r) => r.source === 'overlay')
      const restored = defaultBounceRules.map((r, i) => ({ ...r, id: `br_${i}` }))
      // Rebuild the collection: overwrite by removing all then re-creating.
      all('bounceRules').slice().forEach((r) => removeRow('bounceRules', r.id))
      ;[...restored, ...overlay].forEach((r) => createRow('bounceRules', r))
      return ok({ items: all('bounceRules') })
    },
  },
  {
    method: 'POST',
    pattern: '/bounce-rules:test',
    handler: (ctx) => {
      const body = ctx.body as Record<string, unknown>
      const sig = {
        smtpCode: String(body.smtp_code ?? ''),
        enhancedCode: parseEnhanced(String(body.diagnostic ?? '')),
        provider: providerForDomain(String(body.domain ?? '')),
        diagnostic: String(body.diagnostic ?? ''),
      }
      let best: BounceRule | undefined
      for (const r of all('bounceRules')) {
        if (!matches(r, sig)) continue
        if (!best || r.priority > best.priority || (r.priority === best.priority && specificity(r) > specificity(best))) {
          best = r
        }
      }
      const result: TestBounceDiagnosticResult = {
        smtpCode: sig.smtpCode,
        enhancedCode: sig.enhancedCode,
        provider: sig.provider,
        matched: !!best,
        rule: best,
        effectiveAction: best ? best.action : 'retry',
      }
      return ok(result)
    },
  },
]
