import type { LocationQueryValue } from 'vue-router'

// safeRedirect returns a same-origin path to navigate to after auth, falling
// back to the dashboard and never bouncing back into the auth pages (which
// would loop).
export function safeRedirect(value: LocationQueryValue | LocationQueryValue[]): string {
  const raw = Array.isArray(value) ? value[0] : value
  if (typeof raw !== 'string' || !raw.startsWith('/')) return '/'
  if (raw.startsWith('//')) return '/' // reject protocol-relative URLs
  if (raw.startsWith('/login') || raw.startsWith('/mfa')) return '/'
  return raw
}
