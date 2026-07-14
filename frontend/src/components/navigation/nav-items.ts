import type { Permission } from '@/types'

export interface NavItem {
  label: string
  to?: string
  /** MDI icon name, e.g. 'mdi-send' — rendered on top-level entries. */
  icon?: string
  permission?: Permission
  /** Renders as a collapsible v-list-group. */
  children?: NavItem[]
}

// Intent-based grouping of the old flat sections (see
// docs/vuetify-migration-plan.md — "Better organized menu"). Every `to`
// route and permission is identical to the previous flat menu; only the
// grouping and icons are new.
export const navItems: NavItem[] = [
  {
    label: 'Dashboard',
    to: '/',
    icon: 'mdi-view-dashboard-outline',
    permission: 'dashboard:read',
  },
  {
    label: 'Dashboards',
    to: '/dashboards',
    icon: 'mdi-view-grid-plus-outline',
    permission: 'dashboard:read',
  },
  {
    label: 'Sending',
    icon: 'mdi-email-fast-outline',
    children: [
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
    label: 'Monitoring',
    icon: 'mdi-chart-timeline-variant',
    children: [
      { label: 'Mail Logs', to: '/operations/mail-logs', permission: 'operations:read' },
      { label: 'Bounces', to: '/operations/bounces', permission: 'operations:read' },
      { label: 'Bounce Actions', to: '/operations/bounce-actions', permission: 'operations:read' },
      { label: 'Feedback', to: '/operations/feedback', permission: 'operations:read' },
      { label: 'DMARC Reports', to: '/operations/dmarc', permission: 'operations:read' },
      { label: 'ESP Monitoring', to: '/monitoring/inbox', permission: 'operations:read' },
      { label: 'Queues', to: '/operations/queues', permission: 'operations:read' },
      { label: 'Worker Errors', to: '/operations/worker-errors', permission: 'operations:read' },
    ],
  },
  {
    label: 'Configuration',
    icon: 'mdi-cog-outline',
    children: [
      { label: 'KumoMTA Config', to: '/operations/kumomta-config', permission: 'service:control' },
      { label: 'Cluster Nodes', to: '/operations/cluster', permission: 'service:control' },
      { label: 'Global Settings', to: '/settings/global', permission: 'service:control' },
      {
        label: 'Subject Classifications',
        to: '/settings/subject-classifications',
        permission: 'service:control',
      },
      { label: 'Feedback Loops', to: '/settings/feedback-loops', permission: 'service:control' },
      { label: 'Event Processors', to: '/settings/event-processors', permission: 'service:control' },
      { label: 'System Monitor', to: '/settings/system-monitor', permission: 'service:control' },
      { label: 'Retention', to: '/operations/retention', permission: 'service:control' },
      {
        label: 'Service Control',
        to: '/operations/service-control',
        permission: 'operations:write',
      },
    ],
  },
  {
    label: 'Deliverability',
    icon: 'mdi-shield-check-outline',
    children: [
      { label: 'DKIM Domains', to: '/domain-safety/dkim', permission: 'domain-safety:read' },
      { label: 'TLS Certificates', to: '/operations/acme', permission: 'service:control' },
      { label: 'TLS Policy', to: '/domain-safety/require-tls', permission: 'domain-safety:read' },
      {
        label: 'Suppressions',
        to: '/domain-safety/suppressions',
        permission: 'domain-safety:read',
      },
      {
        label: 'Domain Bounce Readiness',
        to: '/operations/domain-check',
        permission: 'service:control',
      },
    ],
  },
  {
    label: 'Inbound',
    icon: 'mdi-inbox-arrow-down-outline',
    children: [
      { label: 'Inbound Routes', to: '/inbound/routes', permission: 'inbound:read' },
      { label: 'Rspamd Results', to: '/inbound/rspamd', permission: 'inbound:read' },
    ],
  },
  {
    label: 'Tools',
    icon: 'mdi-tools',
    children: [
      { label: 'Diagnose', to: '/tools/diagnose', permission: 'service:control' },
      { label: 'RBL Check', to: '/tools/rbl-check', permission: 'service:control' },
    ],
  },
  {
    label: 'Access',
    icon: 'mdi-shield-account-outline',
    children: [
      { label: 'Users', to: '/security/users', permission: 'security:read' },
      { label: 'MFA & Permissions', to: '/security/access', permission: 'security:read' },
      { label: 'Injection API', to: '/security/injection-credentials', permission: 'security:write' },
      { label: 'Audit Log', to: '/security/audit', permission: 'security:read' },
    ],
  },
]
