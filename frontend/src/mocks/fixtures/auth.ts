// Auth fixture: the admin identity returned by the mock login + /auth:me.
// Any credentials authenticate as this user (MFA is skipped in mock mode).

import type { User } from '../../types'

export const MOCK_TOKEN = 'mock-session-token'

// Mirrors useAuth's ROLE_PERMISSIONS.admin so navigation + route guards behave
// exactly as a real admin session would.
export const ADMIN_PERMISSIONS: string[] = [
  'dashboard:read',
  'outbound:read',
  'outbound:write',
  'operations:read',
  'operations:write',
  'security:read',
  'security:write',
  'domain-safety:read',
  'domain-safety:write',
  'inbound:read',
  'inbound:write',
  'service:control',
]

// 'owner' maps to the coarse 'admin' role in useAuth.roleFromBackend.
export const ADMIN_USER: User = {
  id: 'usr_admin',
  email: 'admin@iris.local',
  displayName: 'Iris Admin',
  status: 'active',
  mfaRequired: false,
  roles: ['owner'],
}
