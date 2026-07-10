// Settings fixtures: deployment-level global settings (singleton), retention
// views, and subject-line classifications.

import type {
  GlobalSettings,
  RetentionView,
  SubjectClassification,
} from '../../types'
import { daysAgo, hoursAgo } from './util'

export const globalSettings: GlobalSettings = {
  rspamdMode: 'tag',
  rspamdUrl: 'http://rspamd:11334',
  egressEhloDomain: 'mta1.example.net',
  logStreamRedisUrl: 'redis://redis:6379',
  esmtpListen: '[::]:25',
  httpListen: '[::]:8000',
  egressRetryInterval: '5m',
  egressMaxRetryInterval: '1h',
  egressMaxAge: '72h',
  pinEgressPerMessage: false,
  bounceDomain: 'bounces.example.net',
  bounceDomainTemplate: 'b-{mailclass}.example.net',
  autoSuppressHardBounces: true,
  softBounceThreshold: 5,
  suppressionTtl: '720h',
  dmarcReportEmail: 'dmarc@example.net',
  adminHttpAddr: '127.0.0.1:8000',
  adminTlsEnabled: true,
  adminTlsCertDomain: 'admin.example.net',
  acmeRenewInterval: '168h',
  acmeRenewBefore: '720h',
  prometheusUrl: 'http://prometheus:9090',
  fblRequireVerification: true,
  inboundMaildirBasePath: '/var/mail/inbound',
  classifySubjects: true,
  classifyModel: 'subject-classifier-v2',
  classifyThreshold: 0.82,
  classifyApiBase: 'http://classifier:8080',
  injectionEnabled: true,
  injectionListenAddr: ':8025',
  injectionPath: '/api/inject',
  injectionTlsEnabled: true,
  injectionTlsCertDomain: 'inject.example.net',
  monitoringFrom: 'probe@monitor.example.com',
  monitoringReconcileLookback: '1h',
  monitoringFetchTimeout: '30s',
  monitoringFetchGiveup: '2h',
  updatedAt: hoursAgo(3),
  updatedBy: 'admin@iris.local',
}

export const retentionViews: RetentionView[] = [
  {
    policy: { tableName: 'log_events', retentionDays: 30, compressAfterDays: 7, enabled: true, updatedAt: daysAgo(2), updatedBy: 'admin@iris.local' },
    label: 'Mail log events',
    hypertable: true,
    chunkCount: 84,
    compressedChunks: 63,
    totalBytes: 4_812_646_400,
    compressedBytes: 1_204_316_160,
    uncompressedBytes: 3_608_330_240,
    oldestData: daysAgo(30),
    newestData: hoursAgo(0),
    lastRun: { id: 'ret_1', tableName: 'log_events', startedAt: hoursAgo(5), finishedAt: hoursAgo(5), chunksCompressed: 4, chunksDropped: 1, bytesBefore: 5_000_000_000, bytesAfter: 4_812_646_400 },
  },
  {
    policy: { tableName: 'feedback_events', retentionDays: 90, compressAfterDays: 14, enabled: true, updatedAt: daysAgo(2), updatedBy: 'admin@iris.local' },
    label: 'Feedback / FBL events',
    hypertable: true,
    chunkCount: 48,
    compressedChunks: 30,
    totalBytes: 412_646_400,
    compressedBytes: 104_316_160,
    uncompressedBytes: 308_330_240,
    oldestData: daysAgo(90),
    newestData: hoursAgo(1),
  },
  {
    policy: { tableName: 'audit_entries', retentionDays: 365, compressAfterDays: 30, enabled: true, updatedAt: daysAgo(10), updatedBy: 'admin@iris.local' },
    label: 'Audit log',
    hypertable: true,
    chunkCount: 12,
    compressedChunks: 6,
    totalBytes: 58_646_400,
    compressedBytes: 14_316_160,
    uncompressedBytes: 44_330_240,
    oldestData: daysAgo(120),
    newestData: hoursAgo(0),
  },
  {
    policy: { tableName: 'dsn_events', retentionDays: 60, compressAfterDays: 10, enabled: false, updatedAt: daysAgo(40), updatedBy: 'admin@iris.local' },
    label: 'DSN events',
    hypertable: true,
    chunkCount: 20,
    compressedChunks: 0,
    totalBytes: 1_012_646_400,
    compressedBytes: 0,
    uncompressedBytes: 1_012_646_400,
    oldestData: daysAgo(60),
    newestData: hoursAgo(2),
  },
]

const SUBJECT_LABELS: Array<[string, string, SubjectClassification['source']]> = [
  ['Your invoice #10432', 'Invoice', 'ai'],
  ['Reset your password', 'Password reset', 'manual'],
  ['Welcome to Iris!', 'Welcome', 'manual'],
  ['50% off this weekend only', 'Promo', 'ai'],
  ['Your order has shipped', 'Receipt', 'ai'],
  ['Security alert: new login', 'Notification', 'manual'],
  ['Confirm your email address', 'Verification', 'manual'],
  ['Your subscription renews soon', 'Billing', 'ai'],
  ['Weekly digest — July edition', 'Newsletter', 'manual'],
  ['Action required: update payment', 'Billing', 'ai'],
]

// A few operator-authored regex rules with explicit priorities. These are
// evaluated before similarity rules of lower priority (higher runs first).
const REGEX_RULES: Array<[string, string, number]> = [
  ['(?i)^\\[SECURITY\\]', 'security', 100],
  ['(?i)\\bunsubscribe\\b', 'unsubscribe', 90],
  ['(?i)^invoice\\s+#?\\d+', 'invoice', 50],
]

// Repeat the base labels a few times so the mock has enough rows to exercise
// the table's pagination (page size starts at 25).
const similarityRules: SubjectClassification[] = Array.from({ length: 31 }, (_, i) => {
  const [subject, label, source] = SUBJECT_LABELS[i % SUBJECT_LABELS.length]
  const round = Math.floor(i / SUBJECT_LABELS.length)
  const displaySubject = round === 0 ? subject : `${subject} (${round + 1})`
  return {
    id: `cls_${i}`,
    subject: displaySubject,
    subjectNormalized: displaySubject.toLowerCase(),
    label,
    source,
    matchType: 'similarity' as const,
    priority: 0,
    hitCount: String(Math.floor(Math.random() * 5000) + 10),
    createdAt: daysAgo((30 - i + 60) % 60),
    updatedAt: daysAgo(i % 7),
  }
})

const regexRules: SubjectClassification[] = REGEX_RULES.map(([pattern, label, priority], i) => ({
  id: `cls_rx_${i}`,
  subject: pattern,
  subjectNormalized: '',
  label,
  source: 'manual',
  matchType: 'regex' as const,
  priority,
  hitCount: String(Math.floor(Math.random() * 800) + 5),
  createdAt: daysAgo(20 - i),
  updatedAt: daysAgo(i % 5),
}))

// Highest priority first — the order the matcher would evaluate them in.
export const classifications: SubjectClassification[] = [...regexRules, ...similarityRules].sort(
  (a, b) => b.priority - a.priority,
)
