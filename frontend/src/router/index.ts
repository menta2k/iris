import { createRouter, createWebHistory } from 'vue-router'
import { routes } from './routes'
import { useAuth } from '@/composables/useAuth'
import { setUnauthorizedHandler } from '@/services/http'

export const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 }
  },
})

// When an authenticated request is rejected (expired/revoked session), the HTTP
// client clears the token and asks us to send the user back to login, keeping
// the attempted path so we can return there after re-auth.
setUnauthorizedHandler(() => {
  const current = router.currentRoute.value
  if (current.meta.public) return
  void router.replace({ name: 'login', query: { redirect: current.fullPath } })
})

// Auth + permission guard. Unauthenticated users are sent to /login (except on
// public routes); authenticated users are kept out of /login. Per-route
// permissions gate the navigation menu; the backend enforces the real rules.
router.beforeEach((to) => {
  const { isAuthenticated, hasPermission } = useAuth()

  if (!to.meta.public && !isAuthenticated.value) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && isAuthenticated.value) {
    return { name: 'dashboard' }
  }

  const required = to.meta.permission
  if (required && !hasPermission(required)) {
    return { name: 'dashboard' }
  }
  return true
})

router.afterEach((to) => {
  const title = to.meta.title
  if (typeof document !== 'undefined') {
    document.title = title ? `${title} · Iris` : 'Iris · KumoMTA Admin'
  }
})
