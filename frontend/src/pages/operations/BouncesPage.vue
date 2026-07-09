<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import PaginationControls from '@/components/common/PaginationControls.vue'
import { Card, CardContent } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { usePagedList } from '@/composables/usePagedList'
import { mailOperationsService } from '@/services'
import { ApiError } from '@/services/http'
import { formatDateTime } from '@/composables/useTimezone'
import type { Bounce, BounceFilters, DsnMessage } from '@/types'

const TYPE_ITEMS = [
  { title: 'All types', value: '' },
  { title: 'Hard', value: 'hard' },
  { title: 'Soft', value: 'soft' },
  { title: 'DSN (async)', value: 'dsn' },
]

const STATE_ITEMS = [
  { title: 'All states', value: '' },
  { title: 'New', value: 'new' },
  { title: 'Processing', value: 'processing' },
  { title: 'Processed', value: 'processed' },
  { title: 'Suppressed', value: 'suppressed' },
  { title: 'Retried', value: 'retried' },
]

// Quick relative windows sent as the API's from_time lower bound.
const TIME_ITEMS = [
  { title: 'Any time', value: 0 },
  { title: 'Last hour', value: 60 * 60_000 },
  { title: 'Last 6 hours', value: 6 * 60 * 60_000 },
  { title: 'Last 24 hours', value: 24 * 60 * 60_000 },
  { title: 'Last 7 days', value: 7 * 24 * 60 * 60_000 },
]

const filters = ref<BounceFilters>({
  recipient: '',
  mailclass: '',
  bounce_type: '',
  classification: '',
  processing_state: '',
})
const timeWindowMs = ref(0)

// v-text-field `clearable` writes null; normalize and add the time bound so
// the loader (which reads filters at call time) always sends clean values.
function buildFilters(): BounceFilters {
  const clean = Object.fromEntries(
    Object.entries(filters.value).map(([k, v]) => [k, v ?? '']),
  ) as BounceFilters
  if (timeWindowMs.value > 0) {
    clean.from_time = new Date(Date.now() - timeWindowMs.value).toISOString()
  }
  return clean
}

const {
  items,
  loading,
  error,
  notImplemented,
  pageSize,
  pageNumber,
  hasPrev,
  hasNext,
  load,
  reload,
  nextPage,
  prevPage,
  setPageSize,
} = usePagedList<Bounce>({
  loader: (page) => mailOperationsService.listBounces(buildFilters(), page),
})

// Text fields apply as you type (debounced); selects apply immediately.
let debounceTimer: ReturnType<typeof setTimeout> | undefined
watch(
  () => [filters.value.recipient, filters.value.mailclass, filters.value.classification],
  () => {
    clearTimeout(debounceTimer)
    debounceTimer = setTimeout(reload, 400)
  },
)
watch(
  () => [filters.value.bounce_type, filters.value.processing_state, timeWindowMs.value],
  () => reload(),
)

const hasActiveFilters = computed(
  () => timeWindowMs.value > 0 || Object.values(filters.value).some((v) => (v ?? '') !== ''),
)

function resetFilters() {
  clearTimeout(debounceTimer)
  filters.value = {
    recipient: '',
    mailclass: '',
    bounce_type: '',
    classification: '',
    processing_state: '',
  }
  timeWindowMs.value = 0
  reload()
}

// ---- Live auto-refresh (keeps the current page; new rows arrive on page 1) ----

const REFRESH_MS = 15_000

const live = ref(false)
let liveTimer: ReturnType<typeof setInterval> | undefined

watch(live, (on) => {
  clearInterval(liveTimer)
  if (on) liveTimer = setInterval(() => {
    if (!loading.value) load()
  }, REFRESH_MS)
})

onBeforeUnmount(() => {
  clearTimeout(debounceTimer)
  clearInterval(liveTimer)
})

// ---- Presentation helpers ----

// Colour-code the processing State so each state reads as a distinct colour:
// processed → green, new → indigo (unhandled/info), processing/pending → amber,
// suppressed → red, retried → neutral grey.
function stateVariant(state: string) {
  switch ((state || '').toLowerCase()) {
    case 'processed':
      return 'success' as const
    case 'new':
      return 'default' as const
    case 'processing':
    case 'pending':
      return 'warning' as const
    case 'suppressed':
      return 'destructive' as const
    case 'retried':
      return 'secondary' as const
    default:
      return 'secondary' as const
  }
}
function stateLabel(state: string) {
  const s = (state || '').toString()
  return s ? s.charAt(0).toUpperCase() + s.slice(1).toLowerCase() : '—'
}

// Hard bounces are permanent failures — flag them so they stand out from
// retryable soft/dsn rows.
function typeVariant(t: string) {
  return (t || '').toLowerCase() === 'hard' ? ('destructive' as const) : ('outline' as const)
}

// ---- CSV export of the current page ----

function csvValue(b: Bounce): string[] {
  return [
    formatDateTime(b.eventTime),
    b.recipient ?? '',
    b.mailclass ?? '',
    b.smtpStatus ?? '',
    b.bounceType ?? '',
    b.classification ?? '',
    b.diagnostic ?? '',
    b.processingState ?? '',
  ]
}

function exportCsv() {
  const esc = (v: string) => `"${v.replace(/"/g, '""')}"`
  const header = ['Time', 'Recipient', 'Mailclass', 'SMTP Status', 'Type', 'Classification', 'Diagnostic', 'State']
  const lines = [
    header.map(esc).join(','),
    ...items.value.map((b) => csvValue(b).map(esc).join(',')),
  ]
  const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `bounces-page-${pageNumber.value}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

// ---- Expandable master-detail row (single-expand) ----

// The full diagnostic is long; the row shows a truncated preview and the
// expanded row the whole payload (plus the raw DSN for dsn-type bounces).
const expandedId = ref<string | null>(null)

// Lazy-loaded raw DSN message(s) for dsn-type bounces, keyed by bounce id.
type DsnState = { loading: boolean; error: string | null; messages: DsnMessage[] }
const dsnByBounce = ref<Record<string, DsnState>>({})

async function loadDsn(b: Bounce) {
  if (dsnByBounce.value[b.id]) return // already loaded/loading
  dsnByBounce.value = { ...dsnByBounce.value, [b.id]: { loading: true, error: null, messages: [] } }
  try {
    const res = await mailOperationsService.listDsnMessages(b.recipient)
    dsnByBounce.value = {
      ...dsnByBounce.value,
      [b.id]: { loading: false, error: null, messages: res.items ?? [] },
    }
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to load DSN message.'
    dsnByBounce.value = { ...dsnByBounce.value, [b.id]: { loading: false, error: msg, messages: [] } }
  }
}

function toggleExpand(b: Bounce) {
  expandedId.value = expandedId.value === b.id ? null : b.id
  if (expandedId.value === b.id && b.bounceType === 'dsn') loadDsn(b)
}
</script>

<template>
  <div>
    <PageHeader title="Bounces" description="Hard and soft bounces captured from delivery attempts." />

    <Card class="mb-4">
      <CardContent class="pa-4">
        <v-row dense>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
              v-model="filters.recipient"
              label="Recipient"
              placeholder="user@gmail.com or gmail.com"
              prepend-inner-icon="mdi-email-outline"
              variant="outlined"
              density="compact"
              hide-details
              clearable
            />
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
              v-model="filters.mailclass"
              label="Mailclass"
              placeholder="marketing"
              prepend-inner-icon="mdi-tag-outline"
              variant="outlined"
              density="compact"
              hide-details
              clearable
            />
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
              v-model="filters.classification"
              label="Classification"
              placeholder="InvalidRecipient"
              prepend-inner-icon="mdi-shape-outline"
              variant="outlined"
              density="compact"
              hide-details
              clearable
            />
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <v-select
              v-model="filters.bounce_type"
              :items="TYPE_ITEMS"
              data-testid="bounce-type"
              label="Type"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <v-select
              v-model="filters.processing_state"
              :items="STATE_ITEMS"
              data-testid="bounce-state"
              label="State"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-select
              v-model="timeWindowMs"
              :items="TIME_ITEMS"
              data-testid="bounce-window"
              label="Time range"
              prepend-inner-icon="mdi-clock-outline"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="2" class="d-flex align-center">
            <v-btn
              variant="outlined"
              color="secondary"
              block
              :disabled="!hasActiveFilters"
              data-testid="reset-filters"
              @click="resetFilters"
            >
              Reset
            </v-btn>
          </v-col>
        </v-row>
      </CardContent>
    </Card>

    <Card>
      <div class="d-flex flex-wrap align-center ga-1 px-4 py-2">
        <span class="text-subtitle-1 font-weight-bold mr-1">Bounces</span>
        <span class="text-caption text-medium-emphasis">
          page {{ pageNumber }}<template v-if="items.length"> · {{ items.length }} rows</template>
        </span>
        <v-spacer />
        <v-switch
          v-model="live"
          label="Live"
          color="primary"
          density="compact"
          hide-details
          class="mr-2 flex-grow-0"
        />
        <v-btn
          icon="mdi-refresh"
          variant="text"
          size="small"
          :loading="loading"
          aria-label="Refresh"
          title="Refresh"
          @click="load"
        />
        <v-btn
          prepend-icon="mdi-download-outline"
          variant="text"
          size="small"
          :disabled="items.length === 0"
          @click="exportCsv"
        >
          CSV
        </v-btn>
      </div>
      <v-divider />
      <v-progress-linear :active="loading" indeterminate color="primary" height="2" />
      <CardContent class="pa-0">
        <!-- Keep the table mounted during refreshes (live tail, as-you-type
             filters); the full-height spinner only covers the first load. -->
        <DataState
          :loading="loading && items.length === 0"
          :error="error"
          :not-implemented="notImplemented"
          :empty="items.length === 0"
          empty-message="No bounces match the selected filters."
        >
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead style="width: 40px" />
                <TableHead>Time</TableHead>
                <TableHead>Recipient</TableHead>
                <TableHead>Mailclass</TableHead>
                <TableHead>SMTP Status</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Classification</TableHead>
                <TableHead>Diagnostic</TableHead>
                <TableHead>State</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <template v-for="b in items" :key="b.id">
                <TableRow class="row-clickable" @click="toggleExpand(b)">
                  <TableCell>
                    <v-icon
                      size="small"
                      icon="mdi-chevron-down"
                      class="expand-icon"
                      :class="expandedId === b.id ? 'expand-icon--open' : ''"
                    />
                  </TableCell>
                  <TableCell class="text-no-wrap text-medium-emphasis">{{ formatDateTime(b.eventTime) }}</TableCell>
                  <TableCell>{{ b.recipient }}</TableCell>
                  <TableCell>{{ b.mailclass }}</TableCell>
                  <TableCell class="font-mono text-caption">{{ b.smtpStatus }}</TableCell>
                  <TableCell>
                    <Badge v-if="b.bounceType" :variant="typeVariant(b.bounceType)">{{ b.bounceType }}</Badge>
                    <span v-else class="text-medium-emphasis">—</span>
                  </TableCell>
                  <TableCell>
                    <Badge v-if="b.classification" variant="outline" class="bounce-classification">{{ b.classification }}</Badge>
                    <span v-else class="text-medium-emphasis">—</span>
                  </TableCell>
                  <TableCell style="max-width: 384px">
                    <span class="d-block text-truncate text-medium-emphasis" :title="b.diagnostic">{{
                      b.diagnostic
                    }}</span>
                  </TableCell>
                  <TableCell><Badge :variant="stateVariant(b.processingState)">{{ stateLabel(b.processingState) }}</Badge></TableCell>
                </TableRow>
                <tr v-if="expandedId === b.id">
                  <td :colspan="9" class="px-4 py-3">
                    <p class="mb-1 text-caption text-uppercase text-medium-emphasis">Full diagnostic</p>
                    <code class="d-block pa-2 rounded border font-mono text-caption text-break">{{
                      b.diagnostic || '—'
                    }}</code>

                    <template v-if="b.bounceType === 'dsn'">
                      <p class="mt-3 mb-1 text-caption text-uppercase text-medium-emphasis">DSN message</p>
                      <div v-if="dsnByBounce[b.id]?.loading" class="text-caption text-medium-emphasis">Loading…</div>
                      <div v-else-if="dsnByBounce[b.id]?.error" class="text-caption text-error">
                        {{ dsnByBounce[b.id]?.error }}
                      </div>
                      <div
                        v-else-if="!dsnByBounce[b.id]?.messages?.length"
                        class="text-caption text-medium-emphasis"
                      >
                        No archived DSN message for this recipient. Only asynchronous bounces captured after
                        this feature shipped are stored.
                      </div>
                      <div v-for="m in dsnByBounce[b.id]?.messages" v-else :key="m.id" class="mt-1">
                        <div class="text-caption text-medium-emphasis">
                          Received {{ formatDateTime(m.receivedAt) }}
                          <span v-if="m.messageId"> · message {{ m.messageId }}</span>
                        </div>
                        <pre class="dsn-raw">{{ m.rawMessage }}</pre>
                      </div>
                    </template>
                  </td>
                </tr>
              </template>
            </TableBody>
          </Table>
        </DataState>
      </CardContent>
    </Card>

    <PaginationControls
      v-if="!notImplemented && (items.length > 0 || hasPrev)"
      :page-number="pageNumber"
      :has-prev="hasPrev"
      :has-next="hasNext"
      :loading="loading"
      :page-size="pageSize"
      @prev="prevPage"
      @next="nextPage"
      @page-size-change="setPageSize"
    />
  </div>
</template>

<style scoped>
.row-clickable {
  cursor: pointer;
}

.expand-icon {
  transition: transform 0.2s ease;
}
.expand-icon--open {
  transform: rotate(180deg);
}

/* Uniform width so the classification badges line up into a tidy column
   (widths otherwise range from ~104px to ~145px). Higher specificity than the
   Badge component's own min-width so this wins. */
:deep(.v-chip.bounce-classification) {
  min-width: 150px;
}

.dsn-raw {
  max-height: 45vh;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--v-font-monospace, monospace);
  font-size: 0.8125rem;
  line-height: 1.4;
  padding: 0.75rem;
  border-radius: 6px;
  background: rgba(var(--v-theme-on-surface), 0.04);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.12);
}
</style>
