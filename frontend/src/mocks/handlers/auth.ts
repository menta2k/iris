// Auth handlers. In mock mode ANY credentials authenticate as the admin user in
// a single step (MFA endpoints reply success but login returns status
// 'authenticated' so the UI never enters the MFA flow).

import { ADMIN_PERMISSIONS, ADMIN_USER, MOCK_TOKEN } from '../fixtures/auth'
import { noContent, ok, type Route, unauthorized } from '../router'

const loginReply = () => ({
  token: MOCK_TOKEN,
  status: 'authenticated' as const,
  user: ADMIN_USER,
  permissions: ADMIN_PERMISSIONS,
})

export const authRoutes: Route[] = [
  {
    method: 'POST',
    pattern: '/auth:login',
    handler: () => ok(loginReply()),
  },
  {
    method: 'POST',
    pattern: '/auth:verify-mfa',
    handler: () => ok(loginReply()),
  },
  {
    method: 'GET',
    pattern: '/auth:me',
    handler: (ctx) => (ctx.token ? ok({ user: ADMIN_USER, permissions: ADMIN_PERMISSIONS }) : unauthorized()),
  },
  {
    method: 'POST',
    pattern: '/mfa:enroll',
    handler: () =>
      ok({
        secret: 'JBSWY3DPEHPK3PXP',
        otpauthUri: 'otpauth://totp/iris:admin@iris.local?secret=JBSWY3DPEHPK3PXP&issuer=iris',
      }),
  },
  {
    method: 'POST',
    pattern: '/mfa:confirm',
    handler: () => ok({ enrolled: true, token: MOCK_TOKEN }),
  },
  {
    method: 'POST',
    pattern: '/mfa:disable',
    handler: () => noContent(),
  },
  {
    method: 'POST',
    pattern: '/auth:change-password',
    handler: () => noContent(),
  },
  {
    method: 'POST',
    pattern: '/auth:logout',
    handler: () => noContent(),
  },
]
