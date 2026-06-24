import type { RouteRecordRaw } from 'vue-router'
import AdminLayout from '@/layouts/AdminLayout.vue'
import type { Permission } from '@/types'

declare module 'vue-router' {
  interface RouteMeta {
    permission?: Permission
    title?: string
    // Routes reachable without an authenticated session (login, MFA steps).
    public?: boolean
  }
}

export const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/pages/auth/LoginPage.vue'),
    meta: { title: 'Sign in', public: true },
  },
  {
    path: '/mfa',
    name: 'mfa',
    component: () => import('@/pages/auth/MfaPage.vue'),
    meta: { title: 'Two-factor authentication', public: true },
  },
  {
    path: '/',
    component: AdminLayout,
    children: [
      {
        path: '',
        name: 'dashboard',
        component: () => import('@/pages/Dashboard.vue'),
        meta: { permission: 'dashboard:read', title: 'Dashboard' },
      },

      // Outbound config
      {
        path: 'outbound/listeners',
        name: 'listeners',
        component: () => import('@/pages/outbound/ListenersPage.vue'),
        meta: { permission: 'outbound:read', title: 'Listeners' },
      },
      {
        path: 'outbound/vmtas',
        name: 'vmtas',
        component: () => import('@/pages/outbound/VmtasPage.vue'),
        meta: { permission: 'outbound:read', title: 'VMTAs' },
      },
      {
        path: 'outbound/vmta-groups',
        name: 'vmta-groups',
        component: () => import('@/pages/outbound/VmtaGroupsPage.vue'),
        meta: { permission: 'outbound:read', title: 'VMTA Groups' },
      },
      {
        path: 'outbound/routing-rules',
        name: 'routing-rules',
        component: () => import('@/pages/outbound/RoutingRulesPage.vue'),
        meta: { permission: 'outbound:read', title: 'Routing Rules' },
      },

      // Operations
      {
        path: 'operations/mail-logs',
        name: 'mail-logs',
        component: () => import('@/pages/operations/MailLogsPage.vue'),
        meta: { permission: 'operations:read', title: 'Mail Logs' },
      },
      {
        path: 'operations/bounces',
        name: 'bounces',
        component: () => import('@/pages/operations/BouncesPage.vue'),
        meta: { permission: 'operations:read', title: 'Bounces' },
      },
      {
        path: 'operations/feedback',
        name: 'feedback',
        component: () => import('@/pages/operations/FeedbackPage.vue'),
        meta: { permission: 'operations:read', title: 'Feedback' },
      },
      {
        path: 'operations/queues',
        name: 'queues',
        component: () => import('@/pages/operations/QueuesPage.vue'),
        meta: { permission: 'operations:read', title: 'Queues' },
      },
      {
        path: 'operations/service-control',
        name: 'service-control',
        component: () => import('@/pages/operations/ServiceControlPage.vue'),
        meta: { permission: 'operations:write', title: 'Service Control' },
      },
      {
        path: 'operations/kumomta-config',
        name: 'kumomta-config',
        component: () => import('@/pages/operations/KumoConfig.vue'),
        meta: { permission: 'service:control', title: 'KumoMTA Config' },
      },
      {
        path: 'settings/global',
        name: 'global-settings',
        component: () => import('@/pages/settings/GlobalSettingsPage.vue'),
        meta: { permission: 'service:control', title: 'Global Settings' },
      },
      {
        path: 'settings/feedback-loops',
        name: 'feedback-loops',
        component: () => import('@/pages/settings/FeedbackLoopsPage.vue'),
        meta: { permission: 'service:control', title: 'Feedback Loops' },
      },
      {
        path: 'operations/acme',
        name: 'acme',
        component: () => import('@/pages/operations/AcmePage.vue'),
        meta: { permission: 'service:control', title: 'TLS Certificates' },
      },
      {
        path: 'operations/domain-check',
        name: 'domain-check',
        component: () => import('@/pages/operations/DomainCheckPage.vue'),
        meta: { permission: 'service:control', title: 'Domain Bounce Readiness' },
      },

      // Domain safety
      {
        path: 'domain-safety/dkim',
        name: 'dkim-domains',
        component: () => import('@/pages/domain-safety/DkimDomainsPage.vue'),
        meta: { permission: 'domain-safety:read', title: 'DKIM Domains' },
      },
      {
        path: 'domain-safety/suppressions',
        name: 'suppressions',
        component: () => import('@/pages/domain-safety/SuppressionsPage.vue'),
        meta: { permission: 'domain-safety:read', title: 'Suppressions' },
      },
      {
        path: 'domain-safety/require-tls',
        name: 'require-tls',
        component: () => import('@/pages/domain-safety/RequireTlsPage.vue'),
        meta: { permission: 'domain-safety:read', title: 'Require TLS' },
      },

      // Inbound automation
      {
        path: 'inbound/webhooks',
        name: 'webhook-rules',
        component: () => import('@/pages/inbound/WebhookRulesPage.vue'),
        meta: { permission: 'inbound:read', title: 'Webhook Rules' },
      },
      {
        path: 'inbound/delivery-events',
        name: 'delivery-events',
        component: () => import('@/pages/inbound/DeliveryEventsPage.vue'),
        meta: { permission: 'inbound:read', title: 'Delivery Events' },
      },
      {
        path: 'inbound/rspamd',
        name: 'rspamd-results',
        component: () => import('@/pages/inbound/RspamdResultsPage.vue'),
        meta: { permission: 'inbound:read', title: 'Rspamd Results' },
      },

      // Security & audit
      {
        path: 'security/users',
        name: 'users',
        component: () => import('@/pages/security/UsersPage.vue'),
        meta: { permission: 'security:read', title: 'Users' },
      },
      {
        path: 'security/access',
        name: 'access',
        component: () => import('@/pages/security/AccessPage.vue'),
        meta: { permission: 'security:read', title: 'MFA & Permissions' },
      },
      {
        path: 'security/audit',
        name: 'audit-log',
        component: () => import('@/pages/security/AuditLogPage.vue'),
        meta: { permission: 'security:read', title: 'Audit Log' },
      },

      {
        path: ':pathMatch(.*)*',
        name: 'not-found',
        component: () => import('@/pages/NotFound.vue'),
      },
    ],
  },
]
