import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from '@/types'

// Mock the auth service so the composable runs without a backend. vi.hoisted
// ensures the mock object exists before the hoisted vi.mock factory runs.
const authMock = vi.hoisted(() => ({
  login: vi.fn(),
  verifyMfa: vi.fn(),
  currentUser: vi.fn(),
  enrollMfa: vi.fn(),
  confirmMfa: vi.fn(),
  logout: vi.fn(),
}))
vi.mock('@/services/auth', () => ({ authService: authMock }))

import { useAuth } from '@/composables/useAuth'
import { http } from '@/services/http'
import { getToken, setToken, clearToken } from '@/services/token'

const owner: User = {
  id: '1',
  email: 'a@example.com',
  displayName: 'Ada',
  status: 'active',
  mfaRequired: false,
  roles: ['owner'],
}

beforeEach(async () => {
  localStorage.clear()
  clearToken()
  vi.clearAllMocks()
  authMock.logout.mockResolvedValue({})
  // Reset shared module-level auth state between tests.
  await useAuth().logout()
})

describe('useAuth', () => {
  it('logs in (authenticated), stores the token, and maps owner → admin', async () => {
    authMock.login.mockResolvedValue({ token: 'tok', status: 'authenticated', user: owner, permissions: ['*'] })
    const a = useAuth()
    const status = await a.login('a@example.com', 'pw')
    expect(status).toBe('authenticated')
    expect(a.isAuthenticated.value).toBe(true)
    expect(getToken()).toBe('tok')
    expect(a.role.value).toBe('admin')
    expect(a.hasPermission('outbound:write')).toBe(true)
  })

  it('keeps the user unauthenticated when MFA is required', async () => {
    authMock.login.mockResolvedValue({ token: 'partial', status: 'mfa_required', user: owner, permissions: [] })
    const a = useAuth()
    const status = await a.login('a@example.com', 'pw')
    expect(status).toBe('mfa_required')
    expect(getToken()).toBe('partial')
    expect(a.isAuthenticated.value).toBe(false)
  })

  it('completes authentication via verifyMfa', async () => {
    authMock.verifyMfa.mockResolvedValue({ token: 'full', status: 'authenticated', user: owner, permissions: ['*'] })
    const a = useAuth()
    await a.verifyMfa('123456')
    expect(a.isAuthenticated.value).toBe(true)
    expect(getToken()).toBe('full')
  })

  it('logout clears the token and user', async () => {
    authMock.login.mockResolvedValue({ token: 'tok', status: 'authenticated', user: owner, permissions: ['*'] })
    const a = useAuth()
    await a.login('a@example.com', 'pw')
    await a.logout()
    expect(a.isAuthenticated.value).toBe(false)
    expect(getToken()).toBeNull()
  })

  it('refresh returns false when there is no token', async () => {
    expect(await useAuth().refresh()).toBe(false)
  })
})

describe('http client auth', () => {
  it('attaches the Bearer token to requests', async () => {
    setToken('xyz')
    const fetchMock = vi.fn().mockResolvedValue(new Response('{}', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await http.get('/users')
    const init = fetchMock.mock.calls[0][1] as RequestInit
    const headers = init.headers as Record<string, string>
    expect(headers.Authorization).toBe('Bearer xyz')
    vi.unstubAllGlobals()
  })

  it('clears the token on a 401 to an authenticated request', async () => {
    setToken('expired')
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ message: 'nope' }), { status: 401 }),
    )
    vi.stubGlobal('fetch', fetchMock)
    await expect(http.get('/users')).rejects.toMatchObject({ status: 401 })
    expect(getToken()).toBeNull()
    vi.unstubAllGlobals()
  })
})
