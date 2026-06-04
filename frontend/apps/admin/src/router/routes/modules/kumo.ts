import type { RouteRecordRaw } from 'vue-router';

import { BasicLayout } from '#/layouts';

const routes: RouteRecordRaw[] = [
  {
    name: 'Delivery',
    path: '/delivery',
    component: BasicLayout,
    meta: {
      icon: 'mdi:email-fast-outline',
      order: 1,
      title: 'Delivery',
    },
    children: [
      {
        name: 'Queues',
        path: '/delivery/queues',
        component: () => import('#/views/kumo/queues/index.vue'),
        meta: { icon: 'lucide:list-tree', title: 'Queues' },
      },
      {
        name: 'Vmtas',
        path: '/delivery/vmtas',
        component: () => import('#/views/kumo/vmtas/index.vue'),
        meta: { icon: 'lucide:server', title: 'Virtual MTAs' },
      },
      {
        name: 'VmtaGroups',
        path: '/delivery/vmta-groups',
        component: () => import('#/views/kumo/vmta-groups/index.vue'),
        meta: { icon: 'lucide:layers', title: 'VMTA Groups' },
      },
      {
        name: 'Routing',
        path: '/delivery/routing',
        component: () => import('#/views/kumo/routing/index.vue'),
        meta: { icon: 'lucide:share-2', title: 'Routing' },
      },
      {
        name: 'MailClasses',
        path: '/delivery/mail-classes',
        component: () => import('#/views/kumo/mail-classes/index.vue'),
        meta: { icon: 'lucide:tags', title: 'Mail Classes' },
      },
      // TODO(backend): /v1/bounces endpoint not yet implemented — hidden from menu.
      {
        name: 'Bounces',
        path: '/delivery/bounces',
        component: () => import('#/views/kumo/bounces/index.vue'),
        meta: {
          icon: 'lucide:undo-2',
          title: 'Bounces',
          hideInMenu: true,
        },
      },
    ],
  },
  {
    name: 'Inbound',
    path: '/inbound',
    component: BasicLayout,
    meta: {
      icon: 'mdi:email-arrow-left-outline',
      order: 2,
      title: 'Inbound',
    },
    children: [
      {
        name: 'Listeners',
        path: '/inbound/listeners',
        component: () => import('#/views/kumo/listener/index.vue'),
        meta: {
          icon: 'lucide:radio-tower',
          title: 'Listeners',
        },
      },
      // TODO(backend): /v1/listener/domains endpoint not yet implemented — hidden from menu.
      {
        name: 'ListenerDomains',
        path: '/inbound/listener-domains',
        component: () => import('#/views/kumo/listener-domains/index.vue'),
        meta: {
          icon: 'lucide:globe',
          title: 'Listener Domains',
          hideInMenu: true,
        },
      },
      {
        name: 'FeedbackReports',
        path: '/inbound/feedback',
        component: () => import('#/views/kumo/feedback/index.vue'),
        meta: { icon: 'lucide:flag', title: 'Feedback Reports' },
      },
    ],
  },
  {
    name: 'Policy',
    path: '/policy',
    component: BasicLayout,
    meta: {
      icon: 'mdi:shield-check-outline',
      order: 3,
      title: 'Policy',
    },
    children: [
      {
        name: 'GlobalSettings',
        path: '/policy/global-settings',
        component: () => import('#/views/kumo/global-settings/index.vue'),
        meta: { icon: 'lucide:settings-2', title: 'Global Settings' },
      },
      {
        name: 'PolicyEditor',
        path: '/policy/editor',
        component: () => import('#/views/kumo/policy/index.vue'),
        meta: { icon: 'lucide:file-code-2', title: 'Policy Editor' },
      },
      {
        name: 'Dkim',
        path: '/policy/dkim',
        component: () => import('#/views/kumo/dkim/index.vue'),
        meta: { icon: 'lucide:key-round', title: 'DKIM Identities' },
      },
      {
        name: 'Suppressions',
        path: '/policy/suppressions',
        component: () => import('#/views/kumo/suppressions/index.vue'),
        meta: { icon: 'lucide:ban', title: 'Suppressions' },
      },
    ],
  },
  {
    name: 'Observability',
    path: '/observability',
    component: BasicLayout,
    meta: {
      icon: 'mdi:chart-line',
      order: 4,
      title: 'Observability',
    },
    children: [
      {
        name: 'Logs',
        path: '/observability/logs',
        component: () => import('#/views/kumo/logs/index.vue'),
        meta: { icon: 'lucide:scroll-text', title: 'Log Stream' },
      },
      {
        name: 'Dsns',
        path: '/observability/dsns',
        component: () => import('#/views/kumo/dsns/index.vue'),
        meta: { icon: 'lucide:mail-x', title: 'Bounces' },
      },
      {
        name: 'AuditLog',
        path: '/observability/audit',
        component: () => import('#/views/kumo/audit/index.vue'),
        meta: { icon: 'lucide:clipboard-list', title: 'Audit Log' },
      },
    ],
  },
  {
    name: 'Security',
    path: '/security',
    component: BasicLayout,
    meta: {
      icon: 'mdi:shield-lock-outline',
      order: 5,
      title: 'Security',
    },
    children: [
      {
        name: 'MfaSettings',
        path: '/security/mfa',
        component: () => import('#/views/kumo/mfa-settings/index.vue'),
        meta: { icon: 'lucide:shield-check', title: 'My MFA' },
      },
      {
        name: 'LoginFirewall',
        path: '/security/login-firewall',
        component: () => import('#/views/kumo/login-firewall/index.vue'),
        meta: { icon: 'lucide:shield-alert', title: 'Login Firewall' },
      },
      {
        name: 'AcmeSettings',
        path: '/security/acme-settings',
        component: () => import('#/views/kumo/acme-settings/index.vue'),
        meta: { icon: 'lucide:settings', title: 'ACME Settings' },
      },
      {
        name: 'AcmeDnsProviders',
        path: '/security/dns-providers',
        component: () => import('#/views/kumo/acme-dns-providers/index.vue'),
        meta: { icon: 'lucide:server', title: 'DNS Providers' },
      },
      {
        name: 'AcmeCertificates',
        path: '/security/certificates',
        component: () => import('#/views/kumo/acme-certificates/index.vue'),
        meta: { icon: 'lucide:badge-check', title: 'Certificates' },
      },
    ],
  },
  {
    name: 'Identity',
    path: '/identity',
    component: BasicLayout,
    meta: {
      icon: 'mdi:account-multiple-outline',
      order: 6,
      title: 'Identity',
      authority: ['admin'],
    },
    children: [
      {
        name: 'Users',
        path: '/identity/users',
        component: () => import('#/views/kumo/users/index.vue'),
        meta: { icon: 'lucide:users', title: 'Users', authority: ['admin'] },
      },
    ],
  },
];

export default routes;
