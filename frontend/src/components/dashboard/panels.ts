// Panel widgets: dashboard widgets backed by iris's own (non-Prometheus) admin
// endpoints, so operational panels that aren't metric time-series — warmup
// delivery health, recent activity feeds, the service summary — can live on a
// custom dashboard alongside metric widgets. Each panel resolves to a table of
// rows the PanelWidget renders generically.
import {
  dashboardService,
  identityAuditService,
  mailOperationsService,
} from '@/services'
import type { WarmupStatsRange } from '@/services/dashboard'

export interface PanelColumn {
  key: string
  label: string
  /** Right-align numeric columns. */
  align?: 'end'
}

export interface PanelRow {
  [key: string]: string
}

export interface PanelDef {
  key: string
  title: string
  category: string
  description: string
  columns: PanelColumn[]
  /** Loads the panel's rows. `range` is the dashboard-wide lookback. */
  load: (ctx: { range: string }) => Promise<PanelRow[]>
}

function fmtTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

function pct(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

// Warmup stats only support 24h/7d; map shorter dashboard ranges up to 24h.
function warmupRange(range: string): WarmupStatsRange {
  return range === '7d' ? '7d' : '24h'
}

export const panelRegistry: PanelDef[] = [
  {
    key: 'service_summary',
    title: 'Service summary',
    category: 'Status',
    description: 'Service state and headline queue/activity counts.',
    columns: [
      { key: 'label', label: 'Metric' },
      { key: 'value', label: 'Value', align: 'end' },
    ],
    load: async () => {
      const s = await dashboardService.getSummary()
      return [
        { label: 'Service state', value: s.serviceState || '—' },
        { label: 'KumoMTA state', value: s.kumoDetail ? `${s.kumoState} (${s.kumoDetail})` : s.kumoState || '—' },
        { label: 'Queued messages', value: s.queuedMessages ?? '0' },
        { label: 'Deferred in queue', value: s.deferredInQueue ?? '0' },
        { label: 'Recent mail events', value: s.recentMailEvents ?? '0' },
        { label: 'Recent audit events', value: s.recentAuditEvents ?? '0' },
      ]
    },
  },
  {
    key: 'warmup_stats',
    title: 'IP warmup delivery',
    category: 'Deliverability',
    description: 'Per-VMTA, per-domain delivery and bounce rates.',
    columns: [
      { key: 'vmta', label: 'VMTA' },
      { key: 'domain', label: 'Domain' },
      { key: 'sent', label: 'Sent', align: 'end' },
      { key: 'bounced', label: 'Bounced', align: 'end' },
      { key: 'delivery', label: 'Deliv.', align: 'end' },
      { key: 'bounce', label: 'Bounce', align: 'end' },
    ],
    load: async ({ range }) => {
      const res = await dashboardService.getWarmupStats(warmupRange(range))
      return (res.rows ?? []).map((r) => ({
        vmta: r.vmtaName || '—',
        domain: r.recipientDomain,
        sent: r.sent,
        bounced: r.bounced,
        delivery: pct(r.deliveryRate),
        bounce: pct(r.bounceRate),
      }))
    },
  },
  {
    key: 'recent_mail',
    title: 'Recent mail activity',
    category: 'Activity',
    description: 'The most recent mail-log events.',
    columns: [
      { key: 'time', label: 'Time' },
      { key: 'recipient', label: 'Recipient' },
      { key: 'status', label: 'Status' },
    ],
    load: async () => {
      const res = await mailOperationsService.listMailRecords()
      return (res.items ?? []).slice(0, 10).map((m) => ({
        time: fmtTime(m.eventTime),
        recipient: m.recipient,
        status: m.status,
      }))
    },
  },
  {
    key: 'recent_audit',
    title: 'Recent audit activity',
    category: 'Activity',
    description: 'The most recent audit-log entries.',
    columns: [
      { key: 'time', label: 'Time' },
      { key: 'operation', label: 'Operation' },
      { key: 'outcome', label: 'Outcome' },
    ],
    load: async () => {
      const res = await identityAuditService.listAuditEntries()
      return (res.items ?? []).slice(0, 10).map((a) => ({
        time: fmtTime(a.occurredAt),
        operation: a.operation,
        outcome: a.outcome,
      }))
    },
  },
]

export function lookupPanel(key: string): PanelDef | undefined {
  return panelRegistry.find((p) => p.key === key)
}
