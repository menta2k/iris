import { computed, ref } from 'vue'
import type { Permission, Role } from '@/types'

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

// In dev, auth is bypassed; default to admin so the full UI is reachable.
const currentRole = ref<Role>('admin')
const currentUser = ref({ email: 'dev@iris.local', display_name: 'Dev Admin' })

export function useAuth() {
  const role = computed(() => currentRole.value)
  const user = computed(() => currentUser.value)
  const permissions = computed(() => ROLE_PERMISSIONS[currentRole.value])

  function hasPermission(permission?: Permission): boolean {
    if (!permission) return true
    return permissions.value.includes(permission)
  }

  function setRole(role: Role) {
    currentRole.value = role
  }

  return { role, user, permissions, hasPermission, setRole }
}
