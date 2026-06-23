<script setup lang="ts">
import { onMounted, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import ServiceStatusWidget from '@/components/dashboard/ServiceStatusWidget.vue'
import QueueHealthWidget from '@/components/dashboard/QueueHealthWidget.vue'
import RecentMailActivity from '@/components/dashboard/RecentMailActivity.vue'
import RecentAuditActivity from '@/components/dashboard/RecentAuditActivity.vue'
import MailFlowPanel from '@/components/dashboard/MailFlowPanel.vue'
import { dashboardService, mailOperationsService, identityAuditService } from '@/services'
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

onMounted(load)
</script>

<template>
  <div>
    <PageHeader title="Dashboard" description="Operational overview of your KumoMTA deployment." />

    <DataState :loading="loading" :error="error" :not-implemented="notImplemented">
      <div class="space-y-6">
        <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <ServiceStatusWidget :state="summary?.serviceState" />
          <QueueHealthWidget :queued="summary?.queuedMessages" />
        </div>
        <MailFlowPanel />
        <div class="grid gap-4 lg:grid-cols-2">
          <RecentMailActivity :events="recentMail" :count="summary?.recentMailEvents" />
          <RecentAuditActivity :events="recentAudit" :count="summary?.recentAuditEvents" />
        </div>
      </div>
    </DataState>
  </div>
</template>
