<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { StatusBadge } from '@/components/ui/badge'
import { formatDateTime } from '@/composables/useTimezone'
import { mailOperationsService } from '@/services'
import type { MailRecord, NextDeliveryAttempt } from '@/types'

// Full-record inspector shown in the Mail Logs right-hand detail drawer.
// `related` is every loaded record sharing the message id (the message's
// lifecycle: reception → delivery/bounce/…), so operators can follow a
// message without leaving the table.
const props = defineProps<{
  record: MailRecord
  related: MailRecord[]
}>()

const emit = defineEmits<{
  (e: 'select', record: MailRecord): void
}>()

interface Field {
  label: string
  value: string
  mono?: boolean
}

function eventMs(r: MailRecord): number {
  return new Date(r.eventTime).getTime()
}

const DELIVERED_STATUS = new Set(['sent', 'delivered'])

// Delivery time = how long the message sat in the queue, from its Reception
// event to its successful Delivery (Sent) event. Derived from the loaded
// lifecycle events for this message; null when either end isn't present.
const deliveryDuration = computed<number | null>(() => {
  const events = props.related.length ? props.related : [props.record]
  let received: number | null = null
  let sent: number | null = null
  for (const r of events) {
    const type = (r.recordType || '').toLowerCase()
    const status = (r.status || '').toLowerCase()
    const t = eventMs(r)
    if (Number.isNaN(t)) continue
    if (type === 'reception' || status === 'received') {
      received = received === null ? t : Math.min(received, t)
    }
    if (type === 'delivery' || DELIVERED_STATUS.has(status)) {
      sent = sent === null ? t : Math.min(sent, t)
    }
  }
  if (received === null || sent === null || sent < received) return null
  return sent - received
})

// Human-readable duration: "480 ms", "3.2 s", "1m 12s", "1h 4m".
function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`
  const s = ms / 1000
  if (s < 60) return `${s < 10 ? s.toFixed(1) : Math.round(s)} s`
  const totalMin = Math.floor(s / 60)
  if (totalMin < 60) return `${totalMin}m ${Math.round(s % 60)}s`
  return `${Math.floor(totalMin / 60)}h ${totalMin % 60}m`
}

const fields = computed<Field[]>(() =>
  [
    { label: 'Event time', value: formatDateTime(props.record.eventTime) },
    {
      label: 'Delivery time (in queue)',
      value: deliveryDuration.value !== null ? formatDuration(deliveryDuration.value) : '',
    },
    { label: 'Message ID', value: props.record.messageId, mono: true },
    { label: 'Record type', value: props.record.recordType || '—' },
    { label: 'Mailclass', value: props.record.mailclass || '—' },
    { label: 'Classification', value: props.record.classification || '—' },
    { label: 'From header', value: props.record.fromHeader || '—' },
    { label: 'Envelope sender', value: props.record.sender || '—', mono: true },
    { label: 'Recipient', value: props.record.recipient || '—', mono: true },
    { label: 'Recipient domain', value: props.record.recipientDomain || '—' },
    { label: 'VMTA', value: props.record.egressSource || props.record.vmtaId || '—', mono: true },
    { label: 'SMTP status', value: props.record.smtpStatus || '—', mono: true },
  ].filter((f) => f.value !== ''),
)

const lifecycle = computed(() =>
  [...props.related].sort((a, b) => a.eventTime.localeCompare(b.eventTime)),
)

// Estimated retry schedule for a deferred message. Fetched from the backend,
// which reconstructs the message's full lifecycle (not just this page) and
// applies the effective exponential-backoff schedule.
const nextAttempt = ref<NextDeliveryAttempt | null>(null)

watch(
  () => props.record.messageId,
  async (messageId) => {
    nextAttempt.value = null
    if (!messageId) return
    try {
      nextAttempt.value = await mailOperationsService.nextDeliveryAttempt(messageId)
    } catch {
      // Best-effort: the estimate is a convenience, never blocks the detail view.
    }
  },
  { immediate: true },
)

const deferredEstimate = computed(() => (nextAttempt.value?.deferred ? nextAttempt.value : null))

// Relative time from now, e.g. "in 1h 4m" / "due now".
function fromNow(iso?: string): string {
  if (!iso) return ''
  const ms = new Date(iso).getTime() - Date.now()
  if (Number.isNaN(ms)) return ''
  return ms <= 0 ? 'due now' : `in ${formatDuration(ms)}`
}
</script>

<template>
  <div class="pa-4">
    <div class="d-flex align-center justify-space-between mb-3">
      <span class="text-subtitle-1 font-weight-bold">Mail record</span>
      <StatusBadge :status="record.status" />
    </div>

    <dl class="detail-grid text-body-2">
      <template v-for="f in fields" :key="f.label">
        <dt class="text-medium-emphasis text-no-wrap">{{ f.label }}</dt>
        <dd :class="{ 'font-mono text-caption': f.mono }" class="text-break">{{ f.value }}</dd>
      </template>
    </dl>

    <template v-if="deferredEstimate">
      <p class="mt-4 mb-1 text-caption text-uppercase text-medium-emphasis">Retry schedule (estimated)</p>
      <div class="rounded border pa-3 text-body-2 d-flex flex-column ga-1">
        <template v-if="deferredEstimate.willExpire">
          <div class="d-flex align-center ga-2">
            <StatusBadge status="bounced" />
            <span>No further attempts — expires {{ formatDateTime(deferredEstimate.expiresAt) }} ({{ fromNow(deferredEstimate.expiresAt) }}).</span>
          </div>
        </template>
        <template v-else>
          <div>
            <span class="text-medium-emphasis">Next attempt:</span>
            <span class="font-weight-medium"> {{ formatDateTime(deferredEstimate.nextAttempt) }}</span>
            <span class="text-medium-emphasis"> ({{ fromNow(deferredEstimate.nextAttempt) }})</span>
          </div>
          <div>
            <span class="text-medium-emphasis">Attempts so far:</span> {{ deferredEstimate.attempts }}
            <span class="text-medium-emphasis ml-3">Remaining before expiry:</span>
            <span class="font-weight-medium"> ~{{ deferredEstimate.remainingAttempts }}</span>
          </div>
          <div class="text-caption text-medium-emphasis">
            Backoff doubles each try; expires {{ formatDateTime(deferredEstimate.expiresAt) }} ({{ fromNow(deferredEstimate.expiresAt) }}).
            Estimate — subject to jitter and traffic shaping.
          </div>
        </template>
      </div>
    </template>

    <template v-if="record.diagnostic">
      <p class="mt-4 mb-1 text-caption text-uppercase text-medium-emphasis">Diagnostic</p>
      <code class="d-block pa-2 rounded border font-mono text-caption text-break">{{
        record.diagnostic
      }}</code>
    </template>

    <template v-if="lifecycle.length > 1">
      <p class="mt-4 mb-1 text-caption text-uppercase text-medium-emphasis">
        Message lifecycle ({{ lifecycle.length }} events on this page)
      </p>
      <v-list density="compact" class="pa-0" bg-color="transparent">
        <v-list-item
          v-for="ev in lifecycle"
          :key="ev.id"
          :active="ev.id === record.id"
          class="px-2"
          @click="emit('select', ev)"
        >
          <template #prepend>
            <StatusBadge :status="ev.status" class="mr-2" />
          </template>
          <v-list-item-title class="text-body-2">
            {{ ev.recordType || ev.status }}
          </v-list-item-title>
          <v-list-item-subtitle class="text-caption">
            {{ formatDateTime(ev.eventTime) }}
          </v-list-item-subtitle>
        </v-list-item>
      </v-list>
    </template>
  </div>
</template>

<style scoped>
.detail-grid {
  display: grid;
  grid-template-columns: auto 1fr;
  column-gap: 16px;
  row-gap: 6px;
}
</style>
