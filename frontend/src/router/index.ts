import { createRouter, createWebHistory } from 'vue-router'
import { routes } from './routes'
import { useAuth } from '@/composables/useAuth'

export const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 }
  },
})

// Permission guard. In dev, auth is bypassed and the default role is admin,
// so this is permissive; it is structured to enforce per-route permissions
// once real auth is wired in.
router.beforeEach((to) => {
  const { hasPermission } = useAuth()
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
