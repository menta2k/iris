<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import StatTile from '@/components/dashboard/StatTile.vue'
import { Card, CardContent } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useToast } from '@/composables/useToast'
import { useEventStream } from '@/composables/useEventStream'
import ProbeDetailDrawer from './ProbeDetailDrawer.vue'
import { monitoringService } from '@/services'
import { ApiError } from '@/services/http'
import type {
  MonitoringAccount,
  MonitoringProbe,
  ProbeAnalysis,
  ProbePlacement,
  ProbeSendStatus,
  SpamVerdict,
} from '@/types'

const route = useRoute()
const router = useRouter()
const { toast } = useToast()

const accountId = computed(() => String(route.params.id))
const account = ref<MonitoringAccount | null>(null)
const probes = ref<MonitoringProbe[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)

// Live updates via SSE: probe status resolves asynchronously (KumoMTA reports
// the outcome seconds–minutes after injection, then the mailbox fetch + header
// analysis land later). When Live is on, the backend pushes each probe
// create/update on the monitoring-probes stream and we upsert it in place.
const live = ref(false)

async function loadAccount() {
  try {
    const res = await monitoringService.listAccounts()
    account.value = (res.items ?? []).find((a) => a.id === accountId.value) ?? null
  } catch {
    account.value = null
  }
}

async function loadProbes() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    const res = await monitoringService.listProbes(accountId.value, { pageSize: 100 })
    probes.value = res.items ?? []
  } catch (err) {
    probes.value = []
    if (err instanceof ApiError && err.notImplemented) {
      notImplemented.value = true
    } else if (err instanceof ApiError && err.status === 0) {
      error.value = 'Cannot reach the backend. Is the API server running?'
    } else {
      error.value = err instanceof Error ? err.message : 'Failed to load probes.'
    }
  } finally {
    loading.value = false
  }
}

// SSE push: upsert the probe (replace by id, or prepend if new) when it belongs
// to the account currently being viewed.
function onProbeEvent(p: MonitoringProbe) {
  if (p.accountId !== accountId.value) return
  const idx = probes.value.findIndex((x) => x.id === p.id)
  if (idx >= 0) {
    probes.value = probes.value.map((x, i) => (i === idx ? p : x))
  } else {
    probes.value = [p, ...probes.value]
  }
}
const probeStream = useEventStream<MonitoringProbe>('monitoring-probes', onProbeEvent)
watch(live, (on) => (on ? probeStream.start() : probeStream.stop()))

// Probe detail drawer: phase timeline, next-phase ETA, event log, inline raw.
const detailOpen = ref(false)
const detailProbe = ref<MonitoringProbe | null>(null)
function openDetail(p: MonitoringProbe) {
  detailProbe.value = p
  detailOpen.value = true
}
// Keep the drawer's probe in sync with SSE upserts while it's open.
watch(
  () => probes.value,
  (list) => {
    if (detailOpen.value && detailProbe.value) {
      const fresh = list.find((x) => x.id === detailProbe.value!.id)
      if (fresh) detailProbe.value = fresh
    }
  },
)

const sending = ref(false)
async function sendProbe() {
  sending.value = true
  try {
    await monitoringService.sendProbe(accountId.value)
    toast({ title: 'Probe sent', description: 'Queued a new probe.', variant: 'success' })
    await loadProbes()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to send probe.'
    toast({ title: 'Send failed', description: msg, variant: 'destructive' })
  } finally {
    sending.value = false
  }
}

// rows augments each probe with its decoded analysis for the template.
const rows = computed(() =>
  probes.value.map((p) => ({ probe: p, analysis: parseAnalysis(p.analysis) })),
)

const stats = computed(() => {
  const list = probes.value
  return {
    total: list.length,
    delivered: list.filter((p) => p.sendStatus === 'sent').length,
    bounced: list.filter((p) => p.sendStatus === 'bounced' || p.sendStatus === 'error').length,
    inbox: list.filter((p) => p.placement === 'inbox').length,
  }
})

const SEND_VARIANT: Record<ProbeSendStatus, 'default' | 'secondary' | 'destructive' | 'success' | 'warning'> = {
  queued: 'secondary',
  sent: 'success',
  deferred: 'warning',
  bounced: 'destructive',
  error: 'destructive',
}
const PLACEMENT_VARIANT: Record<Exclude<ProbePlacement, ''>, 'default' | 'secondary' | 'destructive' | 'success' | 'warning'> = {
  inbox: 'success',
  spam: 'warning',
  missing: 'destructive',
  unknown: 'secondary',
}

const VERDICT_VARIANT: Record<SpamVerdict, 'success' | 'warning' | 'destructive'> = {
  clean: 'success',
  suspicious: 'warning',
  spam: 'destructive',
}

// parseAnalysis safely decodes a probe's analysis JSON (empty/invalid → null).
function parseAnalysis(raw: string): ProbeAnalysis | null {
  if (!raw || raw === '{}') return null
  try {
    const a = JSON.parse(raw) as ProbeAnalysis
    return a && a.verdict ? a : null
  } catch {
    return null
  }
}

function authChips(a: ProbeAnalysis): string {
  return [
    a.spf ? `SPF ${a.spf}` : '',
    a.dkim ? `DKIM ${a.dkim}` : '',
    a.dmarc ? `DMARC ${a.dmarc}` : '',
  ]
    .filter(Boolean)
    .join(' · ')
}

function formatDate(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(undefined, { hour12: false })
}
function formatLatency(ms?: number): string {
  if (ms == null || ms <= 0) return '—'
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(1)} s`
}

onMounted(async () => {
  await Promise.all([loadAccount(), loadProbes()])
})
onBeforeUnmount(() => probeStream.stop())
</script>

<template>
  <div>
    <PageHeader
      :title="account ? `Probes · ${account.label}` : 'Probes'"
      :description="account
        ? `Inbox-placement probes sent to ${account.email}. Send status is reconciled against the mail log; placement is derived from header analysis once the mailbox is fetched.`
        : 'Inbox-placement probes for this mailbox.'"
    >
      <template #actions>
        <div class="d-flex align-center ga-3">
          <div class="d-flex align-center ga-2">
            <v-switch v-model="live" color="primary" density="compact" hide-details inset data-testid="live-toggle" />
            <span class="text-body-2 text-medium-emphasis">Live</span>
          </div>
          <Button variant="outline" @click="router.push({ name: 'inbox-accounts' })">Back</Button>
          <Button :disabled="sending || !(account && account.enabled)" data-testid="send-probe" @click="sendProbe">
            {{ sending ? 'Sending…' : 'Send test' }}
          </Button>
        </div>
      </template>
    </PageHeader>

    <v-row dense class="mb-2">
      <v-col cols="12" sm="6" lg="3">
        <StatTile label="Probes" :value="stats.total.toLocaleString()" caption="Total sent" icon="mdi-email-arrow-right-outline" color="primary" />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile label="Delivered" :value="stats.delivered.toLocaleString()" caption="Accepted by provider" icon="mdi-check-circle-outline" color="success" />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile label="Failed" :value="stats.bounced.toLocaleString()" caption="Bounced or errored" icon="mdi-alert-circle-outline" :color="stats.bounced ? 'error' : 'secondary'" />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile label="Inbox" :value="stats.inbox.toLocaleString()" caption="Landed in inbox" icon="mdi-inbox-arrow-down-outline" color="info" />
      </v-col>
    </v-row>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="probes.length === 0"
      empty-message="No probes yet. Send a test probe to check inbox placement."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Sent</TableHead>
                <TableHead>Probe ID</TableHead>
                <TableHead>Send status</TableHead>
                <TableHead>Mailbox</TableHead>
                <TableHead>Placement</TableHead>
                <TableHead>Spam risk</TableHead>
                <TableHead>Latency</TableHead>
                <TableHead class="text-right">Detail</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="{ probe: p, analysis } in rows" :key="p.id">
                <TableCell class="text-caption text-no-wrap">{{ formatDate(p.sentAt) }}</TableCell>
                <TableCell class="font-mono text-caption">{{ p.probeUid }}</TableCell>
                <TableCell>
                  <Badge :variant="SEND_VARIANT[p.sendStatus]">{{ p.sendStatus }}</Badge>
                  <div v-if="p.error" class="text-caption text-error mt-1">{{ p.error }}</div>
                </TableCell>
                <TableCell class="text-caption text-medium-emphasis">{{ p.mailboxStatus }}</TableCell>
                <TableCell>
                  <Badge v-if="p.placement" :variant="PLACEMENT_VARIANT[p.placement]">{{ p.placement }}</Badge>
                  <span v-else class="text-caption text-medium-emphasis">—</span>
                </TableCell>
                <TableCell>
                  <div v-if="analysis" class="d-flex flex-column ga-1">
                    <div class="d-flex align-center ga-2">
                      <Badge :variant="VERDICT_VARIANT[analysis.verdict!]">{{ analysis.verdict }}</Badge>
                      <span v-if="analysis.source === 'llm'" class="text-caption text-medium-emphasis" title="Assessed by AI header analysis">AI</span>
                    </div>
                    <span v-if="authChips(analysis)" class="text-caption text-medium-emphasis">{{ authChips(analysis) }}</span>
                    <span v-if="analysis.summary" class="text-caption text-medium-emphasis" :title="analysis.summary">{{ analysis.summary }}</span>
                  </div>
                  <span v-else class="text-caption text-medium-emphasis">—</span>
                </TableCell>
                <TableCell class="text-caption text-no-wrap">{{ formatLatency(p.latencyMs) }}</TableCell>
                <TableCell class="text-right">
                  <Button
                    variant="outline"
                    size="sm"
                    :data-testid="`detail-${p.id}`"
                    @click="openDetail(p)"
                  >
                    Details
                  </Button>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>

    <ProbeDetailDrawer v-model:open="detailOpen" :probe="detailProbe" :account="account" />
  </div>
</template>
