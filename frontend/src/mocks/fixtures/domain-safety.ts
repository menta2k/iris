// Domain safety fixtures: DKIM signing domains, suppression list entries, and
// Require-TLS outbound policies.

import type { DkimDomain, Suppression, TLSPolicy } from '../../types'
import { pick, randomString } from './util'

const DAY_MS = 86_400_000
function daysAgoIso(n: number): string {
  return new Date(Date.now() - n * DAY_MS).toISOString()
}
function daysFromNowIso(n: number): string {
  return new Date(Date.now() + n * DAY_MS).toISOString()
}

export const dkimDomains: DkimDomain[] = [
  { id: 'dkim_main', domain: 'example.net', selector: 'iris', publicKeyFingerprint: '8F:2A:1C:9B:44:AE:07:E3', status: 'ready' },
  { id: 'dkim_promo', domain: 'promo.example.net', selector: 'iris', publicKeyFingerprint: '1A:2B:3C:4D:5E:6F:70:81', status: 'ready' },
  { id: 'dkim_news', domain: 'news.example.net', selector: 'mail', publicKeyFingerprint: '9C:8D:7E:6F:50:41:32:23', status: 'needs_attention' },
  { id: 'dkim_alt', domain: 'example.com', selector: 'iris2026', publicKeyFingerprint: 'BB:AA:99:88:77:66:55:44', status: 'disabled' },
]

const baseSuppressions: Suppression[] = [
  { id: 'sup_1', type: 'email', value: 'hard.bounce@example.com', reason: '550 User unknown', source: 'bounce', status: 'active', mailclass: 'transactional', expiresAt: daysFromNowIso(21) },
  { id: 'sup_2', type: 'email', value: 'complainer@example.com', reason: 'FBL complaint', source: 'feedback', status: 'active', mailclass: 'promo' },
  { id: 'sup_3', type: 'domain', value: 'badmail.org', reason: 'Blocklisted provider', source: 'manual', status: 'active' },
  { id: 'sup_4', type: 'email', value: 'invalid@yahoo.com', reason: '550 5.1.1', source: 'bounce', status: 'active' },
  { id: 'sup_5', type: 'email', value: 'expired@outlook.com', reason: '550 5.1.1', source: 'bounce', status: 'active' },
  { id: 'sup_6', type: 'domain', value: 'spamtrap.example', reason: 'Spamtrap hit', source: 'manual', status: 'active' },
  { id: 'sup_7', type: 'email', value: 'temp.user@gmail.com', reason: 'bounce rule: Mailbox Full (persistent) 452', source: 'bounce', status: 'active', mailclass: 'acme_s', expiresAt: daysFromNowIso(30) },
  { id: 'sup_8', type: 'email', value: 'old.contact@example.net', reason: 'Manual removal requested', source: 'manual', status: 'disabled' },
  { id: 'sup_9', type: 'email', value: 'left.company@yahoo.com', reason: '550 5.1.1', source: 'bounce', status: 'active' },
  { id: 'sup_10', type: 'domain', value: 'deadmx.net', reason: 'Persistent delivery failure', source: 'manual', status: 'active' },
  { id: 'sup_dsn', type: 'email', value: 'async.bounce@example.com', reason: 'asynchronous bounce (DSN)', source: 'dsn', status: 'active' },
]

// Suppression lists grow large (hard bounces accumulate); generate a realistic
// backlog so the list demonstrates pagination.
const SUPP_REASONS = ['550 User unknown', '550 5.1.1 Mailbox unavailable', 'FBL complaint', 'Repeated soft bounces', 'Spamtrap hit', 'Blocklisted provider', 'Manual removal requested']
const SUPP_SOURCES = ['bounce', 'bounce', 'bounce', 'feedback', 'manual']
const SUPP_STATUSES = ['active', 'active', 'active', 'active', 'disabled', 'expired']
const SUPP_EMAIL_DOMAINS = ['gmail.com', 'yahoo.com', 'outlook.com', 'icloud.com', 'example.com', 'example.net']
const SUPP_MAILCLASSES = ['newsletter', 'transactional', 'promo', 'acme_s', 'homesbg_h']

const generatedSuppressions: Suppression[] = Array.from({ length: 54 }, (_, i) => {
  const isDomain = i % 9 === 0
  const source = pick(SUPP_SOURCES)
  const status = pick(SUPP_STATUSES)
  return {
    id: `sup_gen_${i}`,
    type: isDomain ? 'domain' : 'email',
    value: isDomain ? `${randomString(6)}.example` : `${randomString(7)}@${pick(SUPP_EMAIL_DOMAINS)}`,
    reason: pick(SUPP_REASONS),
    source,
    status,
    // Event-driven suppressions carry the triggering mailclass; manual ones don't.
    mailclass: source === 'manual' ? '' : pick(SUPP_MAILCLASSES),
    createdAt: daysAgoIso((i % 45) + 1),
    // Auto (bounce/feedback) active entries expire on a TTL; manual ones are permanent.
    expiresAt: source !== 'manual' && status === 'active' ? daysFromNowIso(30 - (i % 20)) : undefined,
  }
})

// Base entries without an explicit createdAt get a staggered recent date.
export const suppressions: Suppression[] = [...baseSuppressions, ...generatedSuppressions].map((s, i) => ({
  createdAt: daysAgoIso((i % 30) + 1),
  ...s,
}))

export const tlsPolicies: TLSPolicy[] = [
  { id: 'tls_gmail', domain: 'gmail.com', mode: 'required', status: 'active' },
  { id: 'tls_finance', domain: 'finance.example.com', mode: 'required', status: 'active' },
  { id: 'tls_outlook', domain: 'outlook.com', mode: 'required_insecure', status: 'active' },
  { id: 'tls_legacy', domain: 'legacy.example.org', mode: 'required_insecure', status: 'inactive' },
]

export const fingerprint = (): string =>
  Array.from({ length: 8 }, () => randomString(2).toUpperCase().padStart(2, '0')).join(':')
