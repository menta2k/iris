// Mail operations + monitoring fixtures: mail records (generated, grouped by
// message-id so the "same message" detail view has rows), bounces, feedback
// reports, live queue summary, worker error logs, and DMARC aggregate reports.

import type {
  Bounce,
  DmarcReport,
  FeedbackReport,
  MailRecord,
  Queue,
  WorkerErrorLog,
} from '../../types'
import { hoursAgo, messageId, minutesAgo, pick, randomString } from './util'

const RECIPIENT_DOMAINS = ['gmail.com', 'yahoo.com', 'outlook.com', 'icloud.com', 'example.com']
const MAILCLASSES = ['transactional', 'promo', 'newsletter']
const SENDERS = ['noreply@example.net', 'news@example.net', 'alerts@example.net']
const FROM_HEADERS = ['Iris Team <noreply@example.net>', 'Weekly Newsletter <news@example.net>', 'Alerts <alerts@example.net>']
const STATUSES: Array<MailRecord['status']> = ['delivered', 'delivered', 'delivered', 'delivered', 'bounced', 'deferred', 'deferred']
const RECORD_BY_STATUS: Record<string, MailRecord['recordType']> = {
  delivered: 'Delivery',
  bounced: 'Bounce',
  deferred: 'TransientFailure',
  received: 'Reception',
  sent: 'Delivery',
}
const CLASSIFICATIONS = ['Invoice', 'Password reset', 'Welcome', 'Promo', 'Receipt', 'Notification', '']

function recipient(): string {
  return `${randomString(6)}@${pick(RECIPIENT_DOMAINS)}`
}

// Build a handful of message-ids, then emit several events per message so the
// Mail Logs detail drawer ("all events for this message") has data to show.
const MESSAGE_IDS = Array.from({ length: 14 }, () => messageId())

export const mailRecords: MailRecord[] = MESSAGE_IDS.flatMap((mid, mi) => {
  const klass = MAILCLASSES[mi % MAILCLASSES.length]
  const sender = SENDERS[mi % SENDERS.length]
  const fromHeader = FROM_HEADERS[mi % FROM_HEADERS.length]
  // One recipient per message; its domain is the recipientDomain so the two
  // columns agree, and all events of the message share the same address.
  const domain = pick(RECIPIENT_DOMAINS)
  const recipientAddr = `${randomString(6)}@${domain}`
  const vmta = `vmta${(mi % 7) + 1}`
  const baseMin = mi * 9 + 2
  return Array.from({ length: 4 }, (_, k) => {
    const status = STATUSES[(mi + k) % STATUSES.length]
    const minutes = baseMin + k * 2
    return {
      id: `mr_${mi}_${k}`,
      messageId: mid,
      eventTime: minutesAgo(minutes),
      mailclass: klass,
      sender,
      fromHeader,
      recipient: recipientAddr,
      recipientDomain: domain,
      vmtaId: vmta,
      egressSource: vmta,
      status,
      recordType: RECORD_BY_STATUS[status] ?? 'Delivery',
      smtpStatus:
        status === 'delivered'
          ? '250 2.0.0 OK'
          : status === 'bounced'
            ? '550 5.1.1 User unknown'
            : '421 4.7.0 Try again later',
      diagnostic:
        status === 'bounced'
          ? 'host said: 550 5.1.1 The email account does not exist'
          : status === 'deferred'
            ? 'host said: 421 4.7.0 Delayed due to rate limiting'
            : '',
      classification: pick(CLASSIFICATIONS),
    } satisfies MailRecord
  })
})

export const bounces: Bounce[] = Array.from({ length: 24 }, (_, i) => {
  const hard = i % 3 !== 0
  return {
    id: `bnc_${i}`,
    eventTime: hoursAgo(i),
    recipient: recipient(),
    mailclass: pick(MAILCLASSES),
    smtpStatus: hard ? '550 5.1.1' : '421 4.7.0',
    bounceType: hard ? 'hard' : 'soft',
    diagnostic: hard ? 'User unknown' : 'Try again later (rate limited)',
    processingState: pick(['new', 'processing', 'suppressed', 'retried']),
    classification: pick(CLASSIFICATIONS),
  } satisfies Bounce
})

export const feedbackReports: FeedbackReport[] = Array.from({ length: 9 }, (_, i) => ({
  id: `fbl_${i}`,
  receivedAt: hoursAgo(i * 3 + 1),
  source: pick(['Yahoo FBL', 'Google FBL', 'Microsoft JMRP']),
  reportType: 'abuse',
  recipient: recipient(),
  processingState: pick(['new', 'verified', 'suppressed']),
  verified: i % 4 !== 0,
  verification: i % 4 !== 0 ? pick(['dkim', 'send-log', 'supplemental-trace']) : '',
} satisfies FeedbackReport))

export const queues: Queue[] = [
  { domain: 'gmail.com', depth: '1284', suspended: false },
  { domain: 'yahoo.com', depth: '342', suspended: false },
  { domain: 'outlook.com', depth: '76', suspended: true, suspendReason: 'High defer rate (manual)' },
  { domain: 'icloud.com', depth: '12', suspended: false },
  { domain: 'example.com', depth: '0', suspended: false },
  { domain: 'legacy.example.org', depth: '408', suspended: true, suspendReason: 'RBL listed' },
]

export const workerErrors: WorkerErrorLog[] = Array.from({ length: 16 }, (_, i) => {
  const isErr = i % 3 === 0
  return {
    id: `we_${i}`,
    eventTime: minutesAgo(i * 7 + 3),
    level: isErr ? 'error' : 'warn',
    worker: pick(['logstream-consumer', 'policy-apply', 'dsn-consumer', 'retention-runner']),
    message: isErr
      ? 'failed to ack redis stream entry'
      : 'retrying after transient upstream error',
    detail: JSON.stringify({ attempt: (i % 5) + 1, stream: 'kumo.events', latencyMs: 12 * (i + 1) }),
  } satisfies WorkerErrorLog
})

export const dmarcDomains: string[] = ['example.net', 'example.com', 'promo.example.net']

export const dmarcReports: DmarcReport[] = Array.from({ length: 9 }, (_, i) => ({
  orgName: pick(['google.com', 'yahoo.com', 'outlook.com']),
  reportId: `report-${randomString(10)}`,
  domain: pick(dmarcDomains),
  dateBegin: hoursAgo((i + 1) * 24),
  dateEnd: hoursAgo(i * 24),
  policyP: pick(['none', 'quarantine', 'reject']),
  policyPct: pick([100, 100, 50]),
  receivedAt: hoursAgo(i * 6 + 2),
} satisfies DmarcReport))
