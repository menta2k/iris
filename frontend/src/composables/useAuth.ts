import { computed, ref } from 'vue'
import type { LoginStatus, Permission, Role, User } from '@/types'
import { authService } from '@/services/auth'
import { clearToken, getToken, setToken } from '@/services/token'

const ROLE_PERMISSIONS: Record<Role, Permission[]> = {
  admin: [
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
  ],
  operator: [
    'dashboard:read',
    'outbound:read',
    'outbound:write',
    'operations:read',
    'operations:write',
    'domain-safety:read',
    'domain-safety:write',
    'inbound:read',
    'inbound:write',
    'security:read',
    'service:control',
  ],
  viewer: [
    'dashboard:read',
    'outbound:read',
    'operations:read',
    'security:read',
    'domain-safety:read',
    'inbound:read',
  ],
}

// Map backend role names to the coarse frontend role used only for navigation
// gating. The backend enforces the real, fine-grained permissions on every API
// call, so this mapping never grants access — it just decides what the UI
// offers.
function roleFromBackend(roles: string[]): Role {
  if (roles.includes('owner') || roles.includes('security_admin')) return 'admin'
  if (roles.includes('operator')) return 'operator'
  return 'viewer'
}

// Shared, module-level auth state.
const currentUser = ref<User | null>(null)
const currentRole = ref<Role>('viewer')
// ready becomes true once a session restore has been attempted at startup, so
// the router guard never redirects before we know whether a token is valid.
const ready = ref(false)

function applyUser(u: User) {
  currentUser.value = u
  currentRole.value = roleFromBackend(u.roles ?? [])
}

function clearSession() {
  clearToken()
  currentUser.value = null
  currentRole.value = 'viewer'
}

export function useAuth() {
  const user = computed(() => currentUser.value)
  const role = computed(() => currentRole.value)
  const permissions = computed(() => ROLE_PERMISSIONS[currentRole.value])
  const isAuthenticated = computed(() => currentUser.value !== null)

  function hasPermission(permission?: Permission): boolean {
    if (!permission) return true
    return permissions.value.includes(permission)
  }

  // login exchanges credentials for a token and returns the status so the
  // caller can route into the MFA flow when required.
  async function login(email: string, password: string): Promise<LoginStatus> {
    const res = await authService.login(email, password)
    setToken(res.token)
    if (res.status === 'authenticated') applyUser(res.user)
    return res.status
  }

  // verifyMfa completes a login that needed a TOTP code.
  async function verifyMfa(code: string): Promise<void> {
    const res = await authService.verifyMfa(code)
    setToken(res.token)
    applyUser(res.user)
  }

  function enrollMfa() {
    return authService.enrollMfa()
  }

  // confirmMfa completes a first-login enrollment, storing the upgraded token
  // and loading the now-authenticated profile.
  async function confirmMfa(code: string): Promise<void> {
    const res = await authService.confirmMfa(code)
    if (res.token) setToken(res.token)
    await refresh()
  }

  // refresh loads the current user from the server using the stored token.
  async function refresh(): Promise<boolean> {
    if (!getToken()) {
      clearSession()
      return false
    }
    try {
      const me = await authService.currentUser()
      applyUser(me.user)
      return true
    } catch {
      clearSession()
      return false
    }
  }

  async function logout(): Promise<void> {
    try {
      await authService.logout()
    } catch {
      // best-effort; the local session is cleared regardless.
    }
    clearSession()
  }

  return {
    user,
    role,
    permissions,
    isAuthenticated,
    ready,
    hasPermission,
    login,
    verifyMfa,
    enrollMfa,
    confirmMfa,
    refresh,
    logout,
  }
}

// restoreSession is called once during app bootstrap, before the router mounts,
// so the first navigation already knows whether a session is active.
export async function restoreSession(): Promise<void> {
  await useAuth().refresh()
  ready.value = true
}
