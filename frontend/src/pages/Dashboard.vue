<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
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
import { dashboardService, mailOperationsService, identityAuditService } from '@/services'
import { useEventStream } from '@/composables/useEventStream'
import type { WarmupStatsRange } from '@/services/dashboard'
import { ApiError } from '@/services/http'
import type { AuditEntry, DashboardSummary, MailRecord } from '@/types'

const summary = ref<DashboardSummary | null>(null)
const recentMail = ref<MailRecord[]>([])
const recentAudit = ref<AuditEntry[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
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
    summary.value = null
    if (err instanceof ApiError && err.notImplemented) {
      notImplemented.value = true
    } else if (err instanceof ApiError && err.status === 0) {
      error.value = 'Cannot reach the backend. Is the API server running?'
    } else {
      error.value = err instanceof Error ? err.message : 'Failed to load dashboard.'
    }
  } finally {
    loading.value = false
  }
}

// Fetchers for the mail-volume panels; map the int64-as-string counts into the
// numeric shape the panel renders.
async function loadMailClassStats(range: WarmupStatsRange): Promise<VolumeRow[]> {
  const res = await dashboardService.getMailClassStats(range)
  return (res.rows ?? []).map((r) => ({
    name: r.mailclass,
    count: Number(r.count),
    delivered: Number(r.delivered),
    bounced: Number(r.bounced),
    deferred: Number(r.deferred),
  }))
}

async function loadRecipientDomainStats(range: WarmupStatsRange): Promise<VolumeRow[]> {
  const res = await dashboardService.getRecipientDomainStats(range)
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
    if (!loading.value) load()
  }, 4000)
}
const dashStream = useEventStream('dashboard', scheduleRefresh)

onMounted(() => {
  load()
  dashStream.start()
})
onBeforeUnmount(() => {
  clearTimeout(refreshTimer)
  dashStream.stop()
})
</script>

<template>
  <div>
    <PageHeader title="Dashboard" description="Operational overview of your KumoMTA deployment." />

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
        <MailFlowPanel />
        <v-row dense>
          <v-col cols="12" lg="6">
            <MailVolumePanel
              title="Mail by Class"
              :fetcher="loadMailClassStats"
              empty-message="No mail in this range yet."
            />
          </v-col>
          <v-col cols="12" lg="6">
            <MailVolumePanel
              title="Top Recipient Domains"
              :fetcher="loadRecipientDomainStats"
              empty-message="No recipient activity in this range yet."
            />
          </v-col>
        </v-row>
        <WarmupStatsPanel />
        <v-row dense>
          <v-col cols="12" lg="8">
            <QueueTimeHistogramPanel />
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
