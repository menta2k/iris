import type { Permission } from '@/types'

export interface NavItem {
  label: string
  to: string
  permission?: Permission
}

export interface NavSection {
  label: string
  items: NavItem[]
}

export const navSections: NavSection[] = [
  {
    label: 'Overview',
    items: [{ label: 'Dashboard', to: '/', permission: 'dashboard:read' }],
  },
  {
    label: 'Outbound Config',
    items: [
      { label: 'Listeners', to: '/outbound/listeners', permission: 'outbound:read' },
      { label: 'VMTAs', to: '/outbound/vmtas', permission: 'outbound:read' },
      { label: 'VMTA Groups', to: '/outbound/vmta-groups', permission: 'outbound:read' },
      { label: 'Routing Rules', to: '/outbound/routing-rules', permission: 'outbound:read' },
      { label: 'IP Warmup', to: '/outbound/warmup', permission: 'outbound:read' },
      { label: 'Delivery Blueprints', to: '/outbound/blueprints', permission: 'outbound:read' },
      { label: 'Shaping Automation', to: '/outbound/automation', permission: 'outbound:read' },
    ],
  },
  {
    label: 'Operations',
    items: [
      { label: 'Mail Logs', to: '/operations/mail-logs', permission: 'operations:read' },
      { label: 'Bounces', to: '/operations/bounces', permission: 'operations:read' },
      { label: 'Feedback', to: '/operations/feedback', permission: 'operations:read' },
      { label: 'DMARC Reports', to: '/operations/dmarc', permission: 'operations:read' },
      { label: 'Queues', to: '/operations/queues', permission: 'operations:read' },
      { label: 'Worker Errors', to: '/operations/worker-errors', permission: 'operations:read' },
      { label: 'Retention', to: '/operations/retention', permission: 'service:control' },
      {
        label: 'Service Control',
        to: '/operations/service-control',
        permission: 'operations:write',
      },
    ],
  },
  {
    label: 'KumoMTA',
    items: [
      {
        label: 'Config',
        to: '/operations/kumomta-config',
        permission: 'service:control',
      },
      {
        label: 'Global Settings',
        to: '/settings/global',
        permission: 'service:control',
      },
      {
        label: 'Feedback Loops',
        to: '/settings/feedback-loops',
        permission: 'service:control',
      },
      {
        label: 'TLS Certificates',
        to: '/operations/acme',
        permission: 'service:control',
      },
      {
        label: 'Domain Bounce Readiness',
        to: '/operations/domain-check',
        permission: 'service:control',
      },
    ],
  },
  {
    label: 'Domain Safety',
    items: [
      { label: 'DKIM Domains', to: '/domain-safety/dkim', permission: 'domain-safety:read' },
      {
        label: 'Suppressions',
        to: '/domain-safety/suppressions',
        permission: 'domain-safety:read',
      },
      { label: 'Require TLS', to: '/domain-safety/require-tls', permission: 'domain-safety:read' },
    ],
  },
  {
    label: 'Inbound Automation',
    items: [
      { label: 'Inbound Routes', to: '/inbound/routes', permission: 'inbound:read' },
      { label: 'Rspamd Results', to: '/inbound/rspamd', permission: 'inbound:read' },
    ],
  },
  {
    label: 'Tools',
    items: [
      { label: 'Diagnose', to: '/tools/diagnose', permission: 'service:control' },
      { label: 'RBL Check', to: '/tools/rbl-check', permission: 'service:control' },
    ],
  },
  {
    label: 'Security & Audit',
    items: [
      { label: 'Users', to: '/security/users', permission: 'security:read' },
      { label: 'MFA & Permissions', to: '/security/access', permission: 'security:read' },
      { label: 'Audit Log', to: '/security/audit', permission: 'security:read' },
    ],
  },
]
