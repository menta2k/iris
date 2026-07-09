import { describe, it, expect } from 'vitest'
import { normalizeSubject, similarity, compileRule, simulate } from '@/utils/subjectMatch'
import type { SubjectClassification } from '@/types'

function rule(p: Partial<SubjectClassification>): SubjectClassification {
  return {
    id: p.id ?? 'x',
    subject: p.subject ?? '',
    subjectNormalized: p.subjectNormalized ?? '',
    label: p.label ?? 'label',
    source: p.source ?? 'manual',
    matchType: p.matchType ?? 'similarity',
    priority: p.priority ?? 0,
    hitCount: p.hitCount ?? '0',
  }
}

describe('normalizeSubject', () => {
  it('mirrors the backend key: strips prefixes, digits, punctuation', () => {
    expect(normalizeSubject('Your order #12345 has shipped')).toBe('your order has shipped')
    expect(normalizeSubject('RE: Re: Password reset')).toBe('password reset')
    expect(normalizeSubject('12345 67890')).toBe('')
  })
})

describe('compileRule', () => {
  it('maps a leading (?i) inline flag to JS flags', () => {
    const re = compileRule('(?i)^invoice')
    expect(re).not.toBeNull()
    expect(re!.test('INVOICE #5')).toBe(true)
  })
  it('returns null for an invalid pattern', () => {
    expect(compileRule('(unclosed')).toBeNull()
  })
})

describe('similarity', () => {
  it('scores identical keys at 1 and unrelated keys low', () => {
    expect(similarity('your order shipped', 'your order shipped')).toBe(1)
    expect(similarity('your order shipped', 'password reset')).toBeLessThan(0.3)
  })
})

describe('simulate', () => {
  it('regex matches the raw subject', () => {
    const r = simulate('INVOICE #42', [
      rule({ id: 'rx', matchType: 'regex', subject: '(?i)^invoice', label: 'invoice', priority: 10 }),
    ])
    expect(r.winner?.rule.id).toBe('rx')
  })

  it('picks the highest priority; a higher-priority similarity rule beats a regex', () => {
    const r = simulate('Your invoice is ready', [
      rule({ id: 'rx', matchType: 'regex', subject: '.', label: 'catchall', priority: 5 }),
      rule({ id: 'sim', matchType: 'similarity', subjectNormalized: 'your invoice is ready', label: 'billing', priority: 10 }),
    ])
    expect(r.winner?.rule.id).toBe('sim')
  })

  it('breaks an equal-priority tie in favour of the regex rule', () => {
    const r = simulate('Your invoice is ready', [
      rule({ id: 'rx', matchType: 'regex', subject: '.', label: 'catchall', priority: 5 }),
      rule({ id: 'sim', matchType: 'similarity', subjectNormalized: 'your invoice is ready', label: 'billing', priority: 5 }),
    ])
    expect(r.winner?.rule.id).toBe('rx')
  })

  it('reports no winner when nothing matches', () => {
    const r = simulate('totally unrelated text', [
      rule({ id: 'rx', matchType: 'regex', subject: '(?i)^invoice', label: 'invoice', priority: 10 }),
    ])
    expect(r.winner).toBeNull()
  })

  it('skips unlabeled (pending) rules', () => {
    const r = simulate('INVOICE #1', [
      rule({ id: 'rx', matchType: 'regex', subject: '(?i)^invoice', label: '', priority: 10 }),
    ])
    expect(r.winner).toBeNull()
  })

  it('collects invalid regex rules separately', () => {
    const r = simulate('anything', [rule({ id: 'bad', matchType: 'regex', subject: '(unclosed', label: 'x' })])
    expect(r.invalid.map((c) => c.id)).toEqual(['bad'])
    expect(r.winner).toBeNull()
  })
})
