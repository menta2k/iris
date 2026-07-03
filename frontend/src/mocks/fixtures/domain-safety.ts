// Domain safety fixtures: DKIM signing domains, suppression list entries, and
// Require-TLS outbound policies.

import type { DkimDomain, Suppression, TLSPolicy } from '../../types'
import { randomString } from './util'

export const dkimDomains: DkimDomain[] = [
  { id: 'dkim_main', domain: 'example.net', selector: 'iris', publicKeyFingerprint: '8F:2A:1C:9B:44:AE:07:E3', status: 'active' },
  { id: 'dkim_promo', domain: 'promo.example.net', selector: 'iris', publicKeyFingerprint: '1A:2B:3C:4D:5E:6F:70:81', status: 'active' },
  { id: 'dkim_news', domain: 'news.example.net', selector: 'mail', publicKeyFingerprint: '9C:8D:7E:6F:50:41:32:23', status: 'needs_attention' },
  { id: 'dkim_alt', domain: 'example.com', selector: 'iris2026', publicKeyFingerprint: 'BB:AA:99:88:77:66:55:44', status: 'disabled' },
]

export const suppressions: Suppression[] = [
  { id: 'sup_1', type: 'email', value: 'hard.bounce@example.com', reason: '550 User unknown', source: 'bounce', status: 'active' },
  { id: 'sup_2', type: 'email', value: 'complainer@example.com', reason: 'FBL complaint', source: 'feedback', status: 'active' },
  { id: 'sup_3', type: 'domain', value: 'badmail.org', reason: 'Blocklisted provider', source: 'manual', status: 'active' },
  { id: 'sup_4', type: 'email', value: 'invalid@yahoo.com', reason: '550 5.1.1', source: 'bounce', status: 'active' },
  { id: 'sup_5', type: 'email', value: 'expired@outlook.com', reason: '550 5.1.1', source: 'bounce', status: 'active' },
  { id: 'sup_6', type: 'domain', value: 'spamtrap.example', reason: 'Spamtrap hit', source: 'manual', status: 'active' },
  { id: 'sup_7', type: 'email', value: 'temp.user@gmail.com', reason: 'Repeated soft bounces', source: 'bounce', status: 'active' },
  { id: 'sup_8', type: 'email', value: 'old.contact@example.net', reason: 'Manual removal requested', source: 'manual', status: 'inactive' },
  { id: 'sup_9', type: 'email', value: 'left.company@yahoo.com', reason: '550 5.1.1', source: 'bounce', status: 'active' },
  { id: 'sup_10', type: 'domain', value: 'deadmx.net', reason: 'Persistent delivery failure', source: 'manual', status: 'active' },
]

export const tlsPolicies: TLSPolicy[] = [
  { id: 'tls_gmail', domain: 'gmail.com', mode: 'required', status: 'active' },
  { id: 'tls_finance', domain: 'finance.example.com', mode: 'required', status: 'active' },
  { id: 'tls_outlook', domain: 'outlook.com', mode: 'required_insecure', status: 'active' },
  { id: 'tls_legacy', domain: 'legacy.example.org', mode: 'required_insecure', status: 'inactive' },
]

export const fingerprint = (): string =>
  Array.from({ length: 8 }, () => randomString(2).toUpperCase().padStart(2, '0')).join(':')
