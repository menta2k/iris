// Identity + audit fixtures: users (admin included) and audit-log entries.

import type { AuditEntry, User } from '../../types'
import { ADMIN_USER } from './auth'
import { hoursAgo, pick, randomString } from './util'

export const users: User[] = [
  ADMIN_USER,
  { id: 'usr_ops', email: 'ops@iris.local', displayName: 'Ops Operator', status: 'active', mfaRequired: false, roles: ['operator'] },
  { id: 'usr_view', email: 'viewer@iris.local', displayName: 'Read-Only Rita', status: 'active', mfaRequired: false, roles: ['viewer'] },
  { id: 'usr_sec', email: 'security@iris.local', displayName: 'Sasha Security', status: 'active', mfaRequired: true, roles: ['security_admin'] },
  { id: 'usr_disabled', email: 'former@iris.local', displayName: 'Former Employee', status: 'disabled', mfaRequired: false, roles: ['viewer'] },
]

const OPERATIONS = [
  'login',
  'create_vmta',
  'update_listener',
  'apply_policy',
  'delete_suppression',
  'create_user',
  'queue_suspend',
  'service_restart',
]
const TARGET_TYPES = ['vmta', 'listener', 'policy', 'suppression', 'user', 'queue', 'routing_rule']
const ACTORS = ['usr_admin', 'usr_ops', 'usr_sec']
const OUTCOMES = ['success', 'success', 'success', 'failure']

export const auditEntries: AuditEntry[] = Array.from({ length: 40 }, (_, i) => {
  const op = pick(OPERATIONS)
  return {
    id: `aud_${randomString(8)}`,
    occurredAt: hoursAgo(i * 0.5),
    actorUserId: pick(ACTORS),
    operation: op,
    targetType: pick(TARGET_TYPES),
    targetId: pick(['vmta3', 'lst_main', 'policy-snapshot', 'sup_4', 'usr_ops', 'gmail.com', 'rule_promo']),
    outcome: pick(OUTCOMES),
    ipAddress: `10.0.0.${10 + (i % 20)}`,
  } satisfies AuditEntry
})
