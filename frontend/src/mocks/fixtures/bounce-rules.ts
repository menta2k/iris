// Bounce-action rules fixture — mirrors the backend's curated default ruleset
// (biz.DefaultBounceRules) so the mock console shows the same starter set.

import type { BounceRule } from '../../types'

type Seed = Omit<BounceRule, 'id' | 'source' | 'status' | 'createdAt' | 'updatedAt'>

const seeds: Seed[] = [
  { smtpCode: '421', enhancedCode: '', provider: '', pattern: '', class: 'soft', category: 'Connection Issue', action: 'retry', actionConfig: '', suggestedAction: 'Retry normally; monitor if frequent.', priority: 50 },
  { smtpCode: '451', enhancedCode: '4.3.0', provider: '', pattern: '', class: 'soft', category: 'Connection Issue', action: 'retry', actionConfig: '', suggestedAction: 'Transient local error; retry.', priority: 50 },
  { smtpCode: '', enhancedCode: '4.4.2', provider: '', pattern: '', class: 'soft', category: 'Connection Issue', action: 'retry', actionConfig: '', suggestedAction: 'Connection dropped; retry.', priority: 50 },
  { smtpCode: '421', enhancedCode: '4.7.0', provider: 'gmail', pattern: 'rate', class: 'soft', category: 'Rate Limited (Too Many Requests)', action: 'throttle', actionConfig: 'receiving/60m', suggestedAction: 'Reduce connection rate and sending speed to Gmail.', priority: 100 },
  { smtpCode: '421', enhancedCode: '', provider: 'yahoo', pattern: 'rate', class: 'soft', category: 'Rate Limited (Too Many Requests)', action: 'throttle', actionConfig: 'receiving/60m', suggestedAction: 'Back off; Yahoo is rate-limiting this IP.', priority: 100 },
  { smtpCode: '', enhancedCode: '4.7.0', provider: 'microsoft', pattern: 'throttl', class: 'soft', category: 'Rate Limited (Too Many Requests)', action: 'throttle', actionConfig: 'receiving/60m', suggestedAction: 'Reduce rate; Outlook/Microsoft throttling.', priority: 100 },
  { smtpCode: '', enhancedCode: '', provider: '', pattern: 'too many', class: 'soft', category: 'Rate Limited (Too Many Requests)', action: 'throttle', actionConfig: 'receiving/30m', suggestedAction: 'Slow down sending to this destination.', priority: 90 },
  { smtpCode: '', enhancedCode: '5.7.1', provider: '', pattern: '', class: 'hard', category: 'Policy / Blocked', action: 'suspend_domain', actionConfig: '2h', suggestedAction: 'Blocked by policy; pause and review. Do not suppress the recipient.', priority: 100 },
  { smtpCode: '550', enhancedCode: '', provider: '', pattern: 'spam', class: 'hard', category: 'Policy / Blocked', action: 'suspend_domain', actionConfig: '2h', suggestedAction: 'Flagged as spam; pause delivery to this destination.', priority: 100 },
  { smtpCode: '554', enhancedCode: '', provider: '', pattern: 'blocked', class: 'hard', category: 'Policy / Blocked', action: 'suspend_domain', actionConfig: '2h', suggestedAction: 'Connection/content blocked; pause and review.', priority: 100 },
  { smtpCode: '', enhancedCode: '5.7.26', provider: '', pattern: '', class: 'hard', category: 'Authentication Failed', action: 'suspend_domain', actionConfig: '1h', suggestedAction: 'SPF/DKIM/DMARC failed; fix authentication for this domain.', priority: 100 },
  { smtpCode: '', enhancedCode: '', provider: '', pattern: 'unauthenticated', class: 'hard', category: 'Authentication Failed', action: 'suspend_domain', actionConfig: '1h', suggestedAction: 'Fix SPF/DKIM authentication for this domain.', priority: 90 },
  { smtpCode: '', enhancedCode: '4.2.2', provider: '', pattern: '', class: 'soft', category: 'Mailbox Full', action: 'retry', actionConfig: '', suggestedAction: 'Mailbox over quota; retry — it often clears.', priority: 80 },
  { smtpCode: '452', enhancedCode: '', provider: '', pattern: 'storage', class: 'soft', category: 'Mailbox Full', action: 'retry', actionConfig: '', suggestedAction: 'Recipient inbox out of storage; retry.', priority: 80 },
  { smtpCode: '550', enhancedCode: '5.1.1', provider: '', pattern: '', class: 'hard', category: 'Invalid Recipient', action: 'suppress', actionConfig: '', suggestedAction: 'Recipient does not exist; suppress the address.', priority: 100 },
  { smtpCode: '', enhancedCode: '5.1.1', provider: '', pattern: 'user unknown', class: 'hard', category: 'Invalid Recipient', action: 'suppress', actionConfig: '', suggestedAction: 'User unknown; suppress the address.', priority: 100 },
  { smtpCode: '', enhancedCode: '5.1.10', provider: '', pattern: '', class: 'hard', category: 'Invalid Recipient', action: 'suppress', actionConfig: '', suggestedAction: 'Address does not exist (NULL MX); suppress.', priority: 100 },
]

export const bounceRules: BounceRule[] = seeds.map((s, i) => ({
  ...s,
  id: `br_${i}`,
  source: 'default',
  status: 'active',
}))
