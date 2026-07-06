// Aggregates every collection fixture into the typed seed data the mock DB loads.
// Handlers import singletons + generators directly from their domain fixture
// files; only the seeded collections live here.

import type { MockData } from '../db'
import { automationRules, blueprints, listeners, routingRules, vmtaGroups, vmtas, warmupSchedules } from './outbound'
import { bounceRules } from './bounce-rules'
import { bounces, dmarcReports, feedbackReports, mailRecords, queues, workerErrors } from './operations'
import { dkimDomains, suppressions, tlsPolicies } from './domain-safety'
import { feedbackLoops, inboundRoutes, rspamdResults } from './inbound'
import { auditEntries, users } from './security'
import { classifications } from './settings'
import { acmeCertificates } from './tools'

export const seedData: MockData = {
  users,
  auditEntries,
  listeners,
  vmtas,
  vmtaGroups,
  routingRules,
  warmupSchedules,
  blueprints,
  automationRules,
  bounceRules,
  mailRecords,
  bounces,
  feedbackReports,
  queues,
  workerErrors,
  dkimDomains,
  suppressions,
  tlsPolicies,
  inboundRoutes,
  rspamdResults,
  feedbackLoops,
  classifications,
  dmarcReports,
  acmeCertificates,
}
