<script setup lang="ts">
import { onMounted, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import ServiceStatusWidget from '@/components/dashboard/ServiceStatusWidget.vue'
import QueueHealthWidget from '@/components/dashboard/QueueHealthWidget.vue'
import RecentMailActivity from '@/components/dashboard/RecentMailActivity.vue'
import RecentAuditActivity from '@/components/dashboard/RecentAuditActivity.vue'
import MailFlowPanel from '@/components/dashboard/MailFlowPanel.vue'
import WarmupStatsPanel from '@/components/dashboard/WarmupStatsPanel.vue'
import MailVolumePanel, { type VolumeRow } from '@/components/dashboard/MailVolumePanel.vue'
import { dashboardService, mailOperationsService, identityAuditService } from '@/services'
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

onMounted(load)
</script>

<template>
  <div>
    <PageHeader title="Dashboard" description="Operational overview of your KumoMTA deployment." />

    <DataState :loading="loading" :error="error" :not-implemented="notImplemented">
      <div class="d-flex flex-column ga-6">
        <v-row dense>
          <v-col cols="12" sm="6" lg="4">
            <ServiceStatusWidget :state="summary?.serviceState" />
          </v-col>
          <v-col cols="12" sm="6" lg="4">
            <QueueHealthWidget :queued="summary?.queuedMessages" />
          </v-col>
        </v-row>
        <MailFlowPanel />
        <v-row dense>
          <v-col cols="12" lg="6">
            <MailVolumePanel
              title="Mail by class"
              :fetcher="loadMailClassStats"
              empty-message="No mail in this range yet."
            />
          </v-col>
          <v-col cols="12" lg="6">
            <MailVolumePanel
              title="Top recipient domains"
              :fetcher="loadRecipientDomainStats"
              empty-message="No recipient activity in this range yet."
            />
          </v-col>
        </v-row>
        <WarmupStatsPanel />
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
