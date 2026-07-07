// Inbound automation fixtures: inbound routes (maildir/forward/webhook), Rspamd
// scan results, and registered feedback loops.

import type {
  FeedbackLoop,
  InboundRoute,
  RspamdResult,
} from '../../types'
import { hoursAgo, pick, randomString } from './util'

export const inboundRoutes: InboundRoute[] = [
  { id: 'ibr_support', name: 'Support inbox', matchType: 'recipient_email', matchValue: 'support@example.net', action: 'maildir', priority: 100, status: 'active', spamScan: 'tag', forwardHost: '', forwardPort: 0, forwardTls: 'none', maildirPath: '/var/mail/support', destinationUrl: '', timeoutSeconds: 30 },
  { id: 'ibr_api', name: 'Inbound API webhook', matchType: 'recipient_domain', matchValue: 'hooks.example.net', action: 'webhook', priority: 90, status: 'active', spamScan: 'enforce', forwardHost: '', forwardPort: 0, forwardTls: 'none', maildirPath: '', destinationUrl: 'https://app.example.net/hooks/inbound', timeoutSeconds: 15 },
  { id: 'ibr_archive', name: 'Archive forward', matchType: 'recipient_email', matchValue: 'archive@example.net', action: 'forward', priority: 50, status: 'active', spamScan: 'default', forwardHost: 'mail.archive.net', forwardPort: 25, forwardTls: 'opportunistic', maildirPath: '', destinationUrl: '', timeoutSeconds: 60 },
  { id: 'ibr_bounce', name: 'Bounce processor', matchType: 'recipient_email', matchValue: 'bounces@example.net', action: 'webhook', priority: 120, status: 'active', spamScan: 'off', forwardHost: '', forwardPort: 0, forwardTls: 'none', maildirPath: '', destinationUrl: 'https://app.example.net/hooks/bounce', timeoutSeconds: 10 },
  { id: 'ibr_old', name: 'Legacy parser', matchType: 'recipient_domain', matchValue: 'old.example.net', action: 'maildir', priority: 10, status: 'disabled', spamScan: 'off', forwardHost: '', forwardPort: 0, forwardTls: 'none', maildirPath: '/var/mail/legacy', destinationUrl: '', timeoutSeconds: 30 },
]

export const rspamdResults: RspamdResult[] = Array.from({ length: 47 }, (_, i) => {
  const spam = i % 4 === 0
  return {
    id: `rsp_${i}`,
    eventTime: hoursAgo(i),
    mailRecordId: `mr_${i % 14}_0`,
    messageId: `<${randomString(10)}@mta1.example.net>`,
    recipient: `${randomString(6)}@${pick(['gmail.com', 'yahoo.com', 'outlook.com'])}`,
    action: spam ? 'reject' : i % 3 === 0 ? 'add header' : 'no action',
    score: spam ? 14.5 + i : -2.1 + i * 0.3,
    symbols: spam
      ? ['BAYES_SPAM', 'RBL_SPAMHAUS', 'MISSING_DATE']
      : ['BAYES_HAM', 'RCVD_DKIM_OK', 'SPF_ALLOW'],
    reason: spam ? 'High spam score' : 'Looks legitimate',
  } satisfies RspamdResult
})

export const feedbackLoops: FeedbackLoop[] = [
  { id: 'fbl_google', domain: 'gmail.com', feedbackAddress: 'feedback+abcd@gmail.com', forwardAddress: 'fbl@example.net', status: 'approved' },
  { id: 'fbl_yahoo', domain: 'yahoo.com', feedbackAddress: 'fbl-return@yahoo.com', forwardAddress: 'fbl@example.net', status: 'approved' },
  { id: 'fbl_outlook', domain: 'outlook.com', feedbackAddress: 'complaints@outlook.com', forwardAddress: 'fbl@example.net', status: 'awaiting_approval' },
  { id: 'fbl_comcast', domain: 'comcast.net', feedbackAddress: 'feedback@comcast.net', forwardAddress: 'fbl@example.net', status: 'awaiting_approval' },
]
