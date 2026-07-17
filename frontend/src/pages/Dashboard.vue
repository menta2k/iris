<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import ServiceStatusWidget from '@/components/dashboard/ServiceStatusWidget.vue'
import QueueHealthWidget from '@/components/dashboard/QueueHealthWidget.vue'
import DeferredQueueWidget from '@/components/dashboard/DeferredQueueWidget.vue'
import MailEventsWidget from '@/components/dashboard/MailEventsWidget.vue'
import RecentMailActivity from '@/components/dashboard/RecentMailActivity.vue'
import RecentAuditActivity from '@/components/dashboard/RecentAuditActivity.vue'
import MailFlowPanel from '@/components/dashboard/MailFlowPanel.vue'
import WarmupStatsPanel from '@/components/dashboard/WarmupStatsPanel.vue'
import SystemStatsPanel from '@/components/dashboard/SystemStatsPanel.vue'
import MailVolumePanel, { type VolumeRow } from '@/components/dashboard/MailVolumePanel.vue'
import QueueTimeHistogramPanel from '@/components/dashboard/QueueTimeHistogramPanel.vue'
import {
  dashboardService,
  mailOperationsService,
  identityAuditService,
  clusterService,
} from '@/services'
import { useEventStream } from '@/composables/useEventStream'
import type { WarmupStatsRange } from '@/services/dashboard'
import { ApiError } from '@/services/http'
import type { AuditEntry, DashboardSummary, MailRecord, MTANode } from '@/types'

// Cluster-node drill-down: '' = all nodes. Populated from the node registry; a
// single-node (non-cluster) deployment simply shows no selector. The selected
// node flows into every panel that supports per-node metrics.
const selectedNode = ref('')
const nodes = ref<MTANode[]>([])
const nodeItems = computed(() => [
  { title: 'All nodes', value: '' },
  ...nodes.value.map((n) => ({ title: n.name, value: n.name })),
])
async function loadNodes() {
  try {
    const res = await clusterService.listNodes()
    nodes.value = (res.items ?? []).filter((n) => n.status !== 'disabled')
  } catch {
    nodes.value = []
  }
}

const summary = ref<DashboardSummary | null>(null)
const recentMail = ref<MailRecord[]>([])
const recentAudit = ref<AuditEntry[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)

async function load(opts: { silent?: boolean } = {}) {
  // The initial load shows the skeleton (loading=true). Event-driven refreshes
  // run silently: toggling `loading` would make DataState unmount/remount the
  // whole panel tree, so every self-fetching sub-panel (mail flow, volume,
  // warmup, …) would refetch and the page would visibly flash every few
  // seconds. A silent refresh updates the KPI tiles + activity feeds in place.
  if (!opts.silent) {
    loading.value = true
    error.value = null
    notImplemented.value = false
  }
  try {
    summary.value = await dashboardService.getSummary()
    // The summary returns counts; fetch the most recent rows for the activity
    // widgets. These are best-effort and must not fail the dashboard.
    const [mail, audit] = await Promise.allSettled([
      mailOperationsService.listMailRecords(),
      identityAuditService.listAuditEntries(),
    ])
    recentMail.value = mail.status === 'fulfilled' ? (mail.value.items ?? []).slice(0, 8) : []
    recentAudit.value = audit.status === 'fulfilled' ? (audit.value.items ?? []).slice(0, 8) : []
  } catch (err) {
    // On a silent refresh keep the last-good view rather than tearing the
    // dashboard down for a transient error; surface it only on a real load.
    if (opts.silent) return
    summary.value = null
    if (err instanceof ApiError && err.notImplemented) {
      notImplemented.value = true
    } else if (err instanceof ApiError && err.status === 0) {
      error.value = 'Cannot reach the backend. Is the API server running?'
    } else {
      error.value = err instanceof Error ? err.message : 'Failed to load dashboard.'
    }
  } finally {
    if (!opts.silent) loading.value = false
  }
}

// Fetchers for the mail-volume panels; map the int64-as-string counts into the
// numeric shape the panel renders.
async function loadMailClassStats(range: WarmupStatsRange, node: string): Promise<VolumeRow[]> {
  const res = await dashboardService.getMailClassStats(range, node)
  return (res.rows ?? []).map((r) => ({
    name: r.mailclass,
    count: Number(r.count),
    delivered: Number(r.delivered),
    bounced: Number(r.bounced),
    deferred: Number(r.deferred),
  }))
}

async function loadRecipientDomainStats(range: WarmupStatsRange, node: string): Promise<VolumeRow[]> {
  const res = await dashboardService.getRecipientDomainStats(range, node)
  return (res.rows ?? []).map((r) => ({
    name: r.recipientDomain,
    count: Number(r.count),
    delivered: Number(r.delivered),
    bounced: Number(r.bounced),
    deferred: Number(r.deferred),
  }))
}

// Real-time: refresh the KPI tiles + recent-activity feeds shortly after new
// mail/bounce events arrive (debounced so a burst collapses into one reload).
let refreshTimer: ReturnType<typeof setTimeout> | undefined
function scheduleRefresh() {
  clearTimeout(refreshTimer)
  refreshTimer = setTimeout(() => {
    if (!loading.value) load({ silent: true })
  }, 4000)
}
const dashStream = useEventStream('dashboard', scheduleRefresh)

onMounted(() => {
  load()
  loadNodes()
  dashStream.start()
})
onBeforeUnmount(() => {
  clearTimeout(refreshTimer)
  dashStream.stop()
})
</script>

<template>
  <div>
    <div class="d-flex align-center justify-space-between flex-wrap ga-3">
      <PageHeader title="Dashboard" description="Operational overview of your KumoMTA deployment." />
      <!-- Cluster node drill-down: only shown when the registry has nodes. -->
      <v-select
        v-if="nodes.length"
        v-model="selectedNode"
        :items="nodeItems"
        data-testid="dashboard-node"
        label="Node"
        variant="outlined"
        density="compact"
        hide-details
        style="max-width: 220px"
      />
    </div>

    <DataState :loading="loading" :error="error" :not-implemented="notImplemented">
      <!-- Widgets are ordered by importance: health KPIs, live mail flow,
           volume breakdowns, warmup performance, then diagnostics and
           activity feeds. -->
      <div class="d-flex flex-column ga-6">
        <v-row dense>
          <v-col cols="12" sm="6" lg="3">
            <ServiceStatusWidget :state="summary?.kumoState || summary?.serviceState" :detail="summary?.kumoDetail" />
          </v-col>
          <v-col cols="12" sm="6" lg="3">
            <QueueHealthWidget :queued="summary?.queuedMessages" />
          </v-col>
          <v-col cols="12" sm="6" lg="3">
            <DeferredQueueWidget :deferred="summary?.deferredInQueue" />
          </v-col>
          <v-col cols="12" sm="6" lg="3">
            <MailEventsWidget :count="summary?.recentMailEvents" />
          </v-col>
        </v-row>
        <MailFlowPanel :node="selectedNode" />
        <v-row dense>
          <v-col cols="12" lg="6">
            <MailVolumePanel
              title="Mail by Class"
              :fetcher="loadMailClassStats"
              :node="selectedNode"
              empty-message="No mail in this range yet."
            />
          </v-col>
          <v-col cols="12" lg="6">
            <MailVolumePanel
              title="Top Recipient Domains"
              :fetcher="loadRecipientDomainStats"
              :node="selectedNode"
              empty-message="No recipient activity in this range yet."
            />
          </v-col>
        </v-row>
        <WarmupStatsPanel :node="selectedNode" />
        <v-row dense>
          <v-col cols="12" lg="8">
            <QueueTimeHistogramPanel :node="selectedNode" />
          </v-col>
          <v-col cols="12" lg="4">
            <SystemStatsPanel />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12" lg="6">
            <RecentMailActivity :events="recentMail" :count="summary?.recentMailEvents" />
          </v-col>
          <v-col cols="12" lg="6">
            <RecentAuditActivity :events="recentAudit" :count="summary?.recentAuditEvents" />
          </v-col>
        </v-row>
      </div>
    </DataState>
  </div>
</template>
