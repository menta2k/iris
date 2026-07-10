<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { monitoringService } from '@/services'
import { ApiError } from '@/services/http'
import type { MonitoringAccount, MonitoringProbe, ProbeAnalysis, ProbeEvent } from '@/types'

const props = defineProps<{
  open: boolean
  probe: MonitoringProbe | null
  account: MonitoringAccount | null
}>()
const emit = defineEmits<{ 'update:open': [boolean] }>()

const events = ref<ProbeEvent[]>([])
const raw = ref<string>('')
const rawTruncated = ref(false)
const loading = ref(false)
const error = ref<string | null>(null)

// Parse the KumoMTA/Go duration form (e.g. "10m", "1h30m", "2h") to ms.
function parseDurationMs(s?: string): number {
  if (!s) return 0
  const units: Record<string, number> = { s: 1e3, m: 60e3, h: 3600e3, d: 86400e3 }
  let total = 0
  for (const m of s.matchAll(/(\d+)([smhd])/g)) total += Number(m[1]) * units[m[2]]
  return total
}

function fmt(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(undefined, { hour12: false })
}

function fmtRelative(ms: number): string {
  const abs = Math.abs(ms)
  const mins = Math.round(abs / 60000)
  if (mins < 1) return ms >= 0 ? 'in <1 min' : 'just now'
  if (mins < 60) return ms >= 0 ? `in ~${mins} min` : `${mins} min ago`
  const hrs = (abs / 3600000).toFixed(1)
  return ms >= 0 ? `in ~${hrs} h` : `${hrs} h ago`
}

const analysis = computed<ProbeAnalysis | null>(() => {
  const a = props.probe?.analysis
  if (!a || a === '{}') return null
  try {
    return JSON.parse(a) as ProbeAnalysis
  } catch {
    return null
  }
})

// The three phases with a resolved status + a human line (incl. next-phase ETA).
interface PhaseStep {
  key: string
  title: string
  state: 'done' | 'active' | 'pending' | 'failed'
  detail: string
}
const phases = computed<PhaseStep[]>(() => {
  const p = props.probe
  if (!p) return []
  const sentMs = p.sentAt ? Date.parse(p.sentAt) : 0
  const delayMs = parseDurationMs(props.account?.fetchDelay || '10m')
  const now = Date.now()

  // Phase 1 — Send
  const send: PhaseStep = { key: 'send', title: '1 · Send', state: 'done', detail: '' }
  if (p.sendStatus === 'sent') send.detail = `Delivered by KumoMTA at ${fmt(p.sentAt)}.`
  else if (p.sendStatus === 'bounced' || p.sendStatus === 'error') {
    send.state = 'failed'
    send.detail = p.error || 'Send failed.'
  } else {
    send.state = 'active'
    send.detail = 'Queued — awaiting KumoMTA delivery confirmation (reconciled against the mail log).'
  }

  // Phase 2 — Fetch
  const fetch: PhaseStep = { key: 'fetch', title: '2 · Mailbox fetch', state: 'pending', detail: '' }
  const fetchAt = sentMs + delayMs
  if (p.mailboxStatus === 'found') {
    fetch.state = 'done'
    fetch.detail = `Found at ${fmt(p.foundAt)}${p.latencyMs ? ` (${Math.round(p.latencyMs / 1000)}s after send)` : ''} → placement: ${p.placement || 'unknown'}.`
  } else if (p.mailboxStatus === 'not_found' || p.mailboxStatus === 'timeout') {
    fetch.state = 'failed'
    fetch.detail = p.error || (p.mailboxStatus === 'timeout' ? 'Mailbox was unreachable.' : 'Not found in the mailbox.')
  } else if (p.sendStatus !== 'sent') {
    fetch.detail = 'Waiting for the send to be confirmed first.'
  } else if (now < fetchAt) {
    fetch.state = 'active'
    fetch.detail = `First mailbox check ${fmtRelative(fetchAt - now)} (${props.account?.fetchDelay || '10m'} after send). ${p.error ? `Last error: ${p.error}` : ''}`
  } else {
    fetch.state = 'active'
    fetch.detail = `Checking the mailbox each minute… ${p.error ? `Last error: ${p.error}` : ''}`
  }

  // Phase 3 — Analyze
  const analyze: PhaseStep = { key: 'analyze', title: '3 · Header analysis', state: 'pending', detail: '' }
  if (analysis.value?.verdict) {
    analyze.state = 'done'
    const a = analysis.value
    analyze.detail = `Spam risk: ${a.verdict}${a.source === 'llm' ? ' (AI)' : ''}. SPF ${a.spf || '—'} · DKIM ${a.dkim || '—'} · DMARC ${a.dmarc || '—'}.`
  } else if (p.mailboxStatus === 'found') {
    analyze.state = 'active'
    analyze.detail = 'Analyzing headers…'
  } else {
    analyze.detail = 'Runs once the message is fetched.'
  }
  return [send, fetch, analyze]
})

const STATE_COLOR: Record<PhaseStep['state'], string> = {
  done: 'success',
  active: 'info',
  pending: 'grey',
  failed: 'error',
}
const STATE_ICON: Record<PhaseStep['state'], string> = {
  done: 'mdi-check-circle',
  active: 'mdi-progress-clock',
  pending: 'mdi-circle-outline',
  failed: 'mdi-alert-circle',
}

async function loadDetail() {
  if (!props.probe) return
  loading.value = true
  error.value = null
  try {
    const [ev, r] = await Promise.all([
      monitoringService.probeEvents(props.probe.id),
      props.probe.mailboxStatus === 'found'
        ? monitoringService.probeRaw(props.probe.id).catch(() => null)
        : Promise.resolve(null),
    ])
    events.value = ev.items ?? []
    const content = r?.rawMessage || r?.rawHeaders || ''
    rawTruncated.value = content.length > 200_000
    raw.value = rawTruncated.value ? content.slice(0, 200_000) : content
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : 'Failed to load probe detail.'
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.open, props.probe?.id, props.probe?.updatedAt],
  () => {
    if (props.open && props.probe) loadDetail()
  },
)

function close() {
  emit('update:open', false)
}
async function copyRaw() {
  try {
    await navigator.clipboard.writeText(raw.value)
  } catch {
    /* clipboard may be blocked; ignore */
  }
}
</script>

<template>
  <v-navigation-drawer
    :model-value="open"
    location="right"
    temporary
    width="620"
    @update:model-value="emit('update:open', $event)"
  >
    <div v-if="probe" class="pa-4 d-flex flex-column ga-4" style="height: 100%; overflow-y: auto">
      <div class="d-flex align-center justify-space-between">
        <div>
          <div class="text-subtitle-1 font-weight-medium">Probe detail</div>
          <div class="text-caption font-mono text-medium-emphasis">{{ probe.probeUid }}</div>
        </div>
        <Button variant="outline" size="sm" @click="close">Close</Button>
      </div>

      <div class="text-caption text-medium-emphasis">
        To <code class="font-mono">{{ probe.recipient }}</code> · sent {{ fmt(probe.sentAt) }}
        <span v-if="probe.messageId"> · msg <code class="font-mono">{{ probe.messageId }}</code></span>
      </div>

      <!-- Phase timeline -->
      <div>
        <div class="text-overline text-medium-emphasis mb-1">Phases</div>
        <div v-for="ph in phases" :key="ph.key" class="d-flex ga-3 py-2">
          <v-icon :icon="STATE_ICON[ph.state]" :color="STATE_COLOR[ph.state]" class="mt-1" />
          <div>
            <div class="d-flex align-center ga-2">
              <span class="text-body-2 font-weight-medium">{{ ph.title }}</span>
              <Badge
                :variant="ph.state === 'failed' ? 'destructive' : ph.state === 'done' ? 'success' : ph.state === 'active' ? 'default' : 'secondary'"
              >
                {{ ph.state }}
              </Badge>
            </div>
            <div class="text-caption text-medium-emphasis">{{ ph.detail }}</div>
          </div>
        </div>
      </div>

      <!-- Event log -->
      <div>
        <div class="d-flex align-center justify-space-between mb-1">
          <div class="text-overline text-medium-emphasis">Event log</div>
          <Button variant="outline" size="sm" :disabled="loading" @click="loadDetail">Refresh</Button>
        </div>
        <div v-if="error" class="text-caption text-error">{{ error }}</div>
        <div v-else-if="!events.length" class="text-caption text-medium-emphasis">No events recorded yet.</div>
        <div v-else class="d-flex flex-column ga-1">
          <div v-for="e in events" :key="e.id" class="d-flex ga-2 align-start">
            <span class="text-caption text-no-wrap text-medium-emphasis" style="min-width: 130px">{{ fmt(e.at) }}</span>
            <Badge :variant="e.level === 'error' ? 'destructive' : 'secondary'">{{ e.phase }}</Badge>
            <span class="text-caption" :class="{ 'text-error': e.level === 'error' }">{{ e.message }}</span>
          </div>
        </div>
      </div>

      <!-- Raw message (inline) -->
      <div v-if="probe.mailboxStatus === 'found'">
        <div class="d-flex align-center justify-space-between mb-1">
          <div class="text-overline text-medium-emphasis">Raw message</div>
          <Button variant="outline" size="sm" :disabled="!raw" @click="copyRaw">Copy</Button>
        </div>
        <pre v-if="raw" class="raw-view">{{ raw }}</pre>
        <div v-else-if="!loading" class="text-caption text-medium-emphasis">No raw message stored.</div>
        <div v-if="rawTruncated" class="text-caption text-warning mt-1">Truncated to 200 KB.</div>
      </div>
    </div>
  </v-navigation-drawer>
</template>

<style scoped>
.raw-view {
  max-height: 320px;
  overflow: auto;
  padding: 12px;
  border-radius: 6px;
  background: rgba(var(--v-theme-on-surface), 0.05);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
