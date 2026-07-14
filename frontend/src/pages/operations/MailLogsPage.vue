<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import PaginationControls from '@/components/common/PaginationControls.vue'
import MailRecordDetail from '@/components/operations/MailRecordDetail.vue'
import { Card, CardContent } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { StatusBadge } from '@/components/ui/badge'
import { usePagedList } from '@/composables/usePagedList'
import { useEventStream } from '@/composables/useEventStream'
import { useConfigStore } from '@/stores/config'
import { useToast } from '@/composables/useToast'
import { mailOperationsService } from '@/services'
import { formatDateTime } from '@/composables/useTimezone'
import type { MailRecord, MailRecordFilters } from '@/types'

// The 7 KumoMTA log record types the log hook streams into mail_records.
const RECORD_TYPES = [
  'Reception',
  'Delivery',
  'Bounce',
  'TransientFailure',
  'AdminBounce',
  'Expiration',
  'Feedback',
] as const

const TYPE_ITEMS = [
  { title: 'All types', value: '' },
  ...RECORD_TYPES.map((t) => ({ title: t, value: t })),
]

// Stored mail-record statuses (backend/internal/biz/mail_record.go).
const STATUS_ITEMS = [
  { title: 'All statuses', value: '' },
  { title: 'Received', value: 'received' },
  { title: 'Sent (delivered)', value: 'sent' },
  { title: 'Deferred', value: 'deferred' },
  { title: 'Bounced', value: 'bounced' },
  { title: 'Suppressed', value: 'suppressed' },
]

// Quick relative windows sent as the API's from_time lower bound.
const TIME_ITEMS = [
  { title: 'Any time', value: 0 },
  { title: 'Last 15 minutes', value: 15 * 60_000 },
  { title: 'Last hour', value: 60 * 60_000 },
  { title: 'Last 6 hours', value: 6 * 60 * 60_000 },
  { title: 'Last 24 hours', value: 24 * 60 * 60_000 },
  { title: 'Last 7 days', value: 7 * 24 * 60 * 60_000 },
]

const filters = ref<MailRecordFilters>({
  mailclass: '',
  sender: '',
  from: '',
  recipient: '',
  vmta_id: '',
  record_type: '',
  status: '',
  diagnostic: '',
  node: '',
})
const timeWindowMs = ref(0)

// v-text-field `clearable` writes null; normalize and add the time bound so
// the loader (which reads filters at call time) always sends clean values.
function buildFilters(): MailRecordFilters {
  const clean = Object.fromEntries(
    Object.entries(filters.value).map(([k, v]) => [k, v ?? '']),
  ) as MailRecordFilters
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
} = usePagedList<MailRecord>({
  loader: (page) => mailOperationsService.listMailRecords(buildFilters(), page),
})

// Text fields apply as you type (debounced); selects apply immediately.
let debounceTimer: ReturnType<typeof setTimeout> | undefined
watch(
  () => [filters.value.mailclass, filters.value.sender, filters.value.from, filters.value.recipient, filters.value.vmta_id, filters.value.diagnostic, filters.value.node],
  () => {
    clearTimeout(debounceTimer)
    debounceTimer = setTimeout(reload, 400)
  },
)
watch(() => [filters.value.record_type, filters.value.status, timeWindowMs.value], () => reload())

const hasActiveFilters = computed(
  () => timeWindowMs.value > 0 || Object.values(filters.value).some((v) => (v ?? '') !== ''),
)

function resetFilters() {
  clearTimeout(debounceTimer)
  filters.value = {
    mailclass: '',
    sender: '',
    from: '',
    recipient: '',
    vmta_id: '',
    record_type: '',
    status: '',
    diagnostic: '',
    node: '',
  }
  timeWindowMs.value = 0
  reload()
}

// ---- Live updates via SSE (matching rows are prepended on page 1) ----

const live = ref(false)
const MAX_LIVE_ROWS = 200

// Client-side filter match mirroring the backend query, so a pushed record only
// appears when it fits the operator's current filters.
function matchesFilters(rec: MailRecord): boolean {
  const f = filters.value
  const has = (v?: string) => (v ?? '').trim() !== ''
  const sub = (hay: string | undefined, needle?: string) =>
    !has(needle) || (hay ?? '').toLowerCase().includes((needle ?? '').toLowerCase())
  if (has(f.mailclass) && rec.mailclass !== f.mailclass) return false
  if (has(f.status) && rec.status !== f.status) return false
  if (has(f.record_type) && rec.recordType !== f.record_type) return false
  if (has(f.node) && rec.node !== f.node) return false
  return sub(rec.sender, f.sender) && sub(rec.fromHeader, f.from) &&
    sub(rec.recipient, f.recipient) && sub(rec.egressSource, f.vmta_id) &&
    sub(rec.diagnostic, f.diagnostic)
}

function onMailEvent(rec: MailRecord) {
  if (pageNumber.value !== 1) return // don't disturb a paginated view
  if (!matchesFilters(rec)) return
  if (items.value.some((m) => m.id === rec.id)) return
  items.value = [rec, ...items.value].slice(0, Math.max(pageSize.value, MAX_LIVE_ROWS))
}

const mailStream = useEventStream<MailRecord>('mail-logs', onMailEvent)
watch(live, (on) => (on ? mailStream.start() : mailStream.stop()))

// ---- Column visibility & density (persisted per-table in the config store) ----

const TABLE_ID = 'mail-logs'

const ALL_COLUMNS = [
  { key: 'time', title: 'Time' },
  { key: 'messageId', title: 'Message ID' },
  { key: 'mailclass', title: 'Mailclass' },
  { key: 'from', title: 'From' },
  { key: 'sender', title: 'Sender (envelope)' },
  { key: 'recipient', title: 'Recipient' },
  { key: 'recipientDomain', title: 'Recipient Domain' },
  { key: 'vmta', title: 'VMTA' },
  { key: 'node', title: 'Node' },
  { key: 'status', title: 'Status' },
  { key: 'type', title: 'Type' },
  { key: 'class', title: 'Class' },
  { key: 'reason', title: 'Reason' },
] as const

type ColumnKey = (typeof ALL_COLUMNS)[number]['key']

// Hidden until the operator opts in: low-signal columns that used to force a
// horizontal scrollbar (domain repeats the recipient; the id is copyable from
// the drawer and the Message ID column once re-enabled).
const DEFAULT_HIDDEN: string[] = ['messageId', 'recipientDomain', 'class', 'node']

const config = useConfigStore()

const hiddenColumns = computed<string[]>(() => config.tablePrefs[TABLE_ID]?.hidden ?? DEFAULT_HIDDEN)
const density = computed(() => config.tablePrefs[TABLE_ID]?.density ?? 'compact')

function isVisible(key: ColumnKey): boolean {
  return !hiddenColumns.value.includes(key)
}

function toggleColumn(key: ColumnKey) {
  const hidden = hiddenColumns.value.includes(key)
    ? hiddenColumns.value.filter((k) => k !== key)
    : [...hiddenColumns.value, key]
  config.setTablePrefs(TABLE_ID, { hidden })
}

function toggleDensity() {
  config.setTablePrefs(TABLE_ID, {
    density: density.value === 'compact' ? 'default' : 'compact',
  })
}

const visibleCount = computed(() => ALL_COLUMNS.filter((c) => isVisible(c.key)).length)

// ---- Cell helpers ----

const { toast } = useToast()

// `"Example" <no-reply@example.com>` -> "Example" (fall back to the address); the
// full header stays available as the cell tooltip.
function fromDisplay(header?: string): string {
  if (!header) return ''
  const match = header.match(/^\s*"?([^"<]*?)"?\s*<(.+)>\s*$/)
  if (match) return match[1].trim() || match[2].trim()
  return header
}

function shortId(id?: string): string {
  if (!id) return ''
  return id.length > 12 ? `${id.slice(0, 12)}…` : id
}

async function copyMessageId(m: MailRecord) {
  try {
    await navigator.clipboard.writeText(m.messageId)
    toast({ title: 'Message ID copied', variant: 'success', duration: 2000 })
  } catch {
    toast({ title: 'Could not copy to clipboard', variant: 'destructive' })
  }
}

// ---- CSV export of the current page (visible columns only) ----

function cellValue(m: MailRecord, key: ColumnKey): string {
  switch (key) {
    case 'time': return formatDateTime(m.eventTime)
    case 'messageId': return m.messageId ?? ''
    case 'mailclass': return m.mailclass ?? ''
    case 'from': return m.fromHeader ?? ''
    case 'sender': return m.sender ?? ''
    case 'recipient': return m.recipient ?? ''
    case 'recipientDomain': return m.recipientDomain ?? ''
    case 'vmta': return m.egressSource || m.vmtaId || ''
    case 'node': return m.node ?? ''
    case 'status': return m.status ?? ''
    case 'type': return m.recordType ?? ''
    case 'class': return m.classification ?? ''
    case 'reason': return `${m.smtpStatus ?? ''} ${m.diagnostic ?? ''}`.trim()
  }
}

function exportCsv() {
  const cols = ALL_COLUMNS.filter((c) => isVisible(c.key))
  const esc = (v: string) => `"${v.replace(/"/g, '""')}"`
  const lines = [
    cols.map((c) => esc(c.title)).join(','),
    ...items.value.map((m) => cols.map((c) => esc(cellValue(m, c.key))).join(',')),
  ]
  const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `mail-logs-page-${pageNumber.value}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

// ---- Right-hand detail drawer (inspection + compare, deep-linked) ----

const route = useRoute()
const router = useRouter()

const selected = ref<MailRecord | null>(null)
const detailOpen = computed({
  get: () => selected.value !== null,
  set: (open: boolean) => {
    if (!open) select(null)
  },
})

const related = computed(() =>
  selected.value
    ? items.value.filter((m) => m.messageId && m.messageId === selected.value?.messageId)
    : [],
)

function select(record: MailRecord | null) {
  selected.value = record
  const query = { ...route.query }
  if (record) query.record = record.id
  else delete query.record
  router.replace({ query })
}

// Deep link: restore ?record=<id> once records arrive (e.g. shared URL).
watch(items, (list) => {
  const id = route.query.record
  if (typeof id === 'string' && !selected.value) {
    const match = list.find((m) => m.id === id)
    if (match) selected.value = match
  }
})

function onEsc(e: KeyboardEvent) {
  if (e.key === 'Escape' && selected.value) select(null)
}
onMounted(() => window.addEventListener('keydown', onEsc))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onEsc)
  clearTimeout(debounceTimer)
  mailStream.stop()
})
</script>

<template>
  <div>
    <PageHeader title="Mail Logs" description="Searchable record of message-level delivery events." />

    <Card class="mb-4">
      <CardContent class="pa-4">
        <v-row dense>
          <v-col cols="12" sm="6" md="3">
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
          <v-col cols="12" sm="6" md="3">
            <v-text-field
              v-model="filters.from"
              label="From header"
              placeholder="sentry@infra.example.com"
              prepend-inner-icon="mdi-account-arrow-right-outline"
              variant="outlined"
              density="compact"
              hide-details
              clearable
            />
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <v-text-field
              v-model="filters.sender"
              label="Sender (envelope)"
              placeholder="news@example.com or example.com"
              prepend-inner-icon="mdi-email-arrow-right-outline"
              variant="outlined"
              density="compact"
              hide-details
              clearable
            />
          </v-col>
          <v-col cols="12" sm="6" md="3">
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
          <v-col cols="12" sm="6" md="3">
            <v-text-field
              v-model="filters.vmta_id"
              label="VMTA"
              placeholder="vmta-1"
              prepend-inner-icon="mdi-server-network-outline"
              variant="outlined"
              density="compact"
              hide-details
              clearable
            />
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <v-text-field
              v-model="filters.node"
              label="Node"
              placeholder="mta-eu-2"
              prepend-inner-icon="mdi-server-outline"
              variant="outlined"
              density="compact"
              hide-details
              clearable
              data-testid="filter-node"
            />
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <v-text-field
              v-model="filters.diagnostic"
              label="Diagnostic"
              placeholder="quota, STARTTLS, NXDOMAIN…"
              prepend-inner-icon="mdi-message-alert-outline"
              variant="outlined"
              density="compact"
              hide-details
              clearable
            />
          </v-col>
          <v-col cols="12" sm="6" md="2">
            <v-select
              v-model="filters.record_type"
              :items="TYPE_ITEMS"
              data-testid="mail-record-type"
              label="Type"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="2">
            <v-select
              v-model="filters.status"
              :items="STATUS_ITEMS"
              data-testid="mail-record-status"
              label="Status"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <v-select
              v-model="timeWindowMs"
              :items="TIME_ITEMS"
              data-testid="mail-record-window"
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
        <span class="text-subtitle-1 font-weight-bold mr-1">Records</span>
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
        <v-btn
          :icon="density === 'compact' ? 'mdi-arrow-expand-vertical' : 'mdi-arrow-collapse-vertical'"
          variant="text"
          size="small"
          :aria-label="density === 'compact' ? 'Comfortable rows' : 'Compact rows'"
          :title="density === 'compact' ? 'Comfortable rows' : 'Compact rows'"
          @click="toggleDensity"
        />
        <v-menu :close-on-content-click="false">
          <template #activator="{ props: menuProps }">
            <v-btn
              v-bind="menuProps"
              prepend-icon="mdi-view-column-outline"
              variant="text"
              size="small"
            >
              Columns ({{ visibleCount }}/{{ ALL_COLUMNS.length }})
            </v-btn>
          </template>
          <v-list density="compact">
            <v-list-item v-for="col in ALL_COLUMNS" :key="col.key" @click="toggleColumn(col.key)">
              <template #prepend>
                <v-checkbox-btn :model-value="isVisible(col.key)" density="compact" />
              </template>
              <v-list-item-title class="text-body-2">{{ col.title }}</v-list-item-title>
            </v-list-item>
          </v-list>
        </v-menu>
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
          empty-message="No mail records match the selected filters."
        >
          <Table :density="density" fixed-header height="calc(100vh - 430px)">
            <TableHeader>
              <TableRow>
                <TableHead v-if="isVisible('time')">Time</TableHead>
                <TableHead v-if="isVisible('messageId')">Message ID</TableHead>
                <TableHead v-if="isVisible('mailclass')">Mailclass</TableHead>
                <TableHead v-if="isVisible('from')">From</TableHead>
                <TableHead v-if="isVisible('sender')">Sender (envelope)</TableHead>
                <TableHead v-if="isVisible('recipient')">Recipient</TableHead>
                <TableHead v-if="isVisible('recipientDomain')">Recipient Domain</TableHead>
                <TableHead v-if="isVisible('vmta')">VMTA</TableHead>
                <TableHead v-if="isVisible('node')">Node</TableHead>
                <TableHead v-if="isVisible('status')">Status</TableHead>
                <TableHead v-if="isVisible('type')">Type</TableHead>
                <TableHead v-if="isVisible('class')">Class</TableHead>
                <TableHead v-if="isVisible('reason')">Reason</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow
                v-for="m in items"
                :key="m.id"
                :class="selected?.id === m.id ? 'row-clickable row-selected' : 'row-clickable'"
                @click.stop="select(m)"
              >
                <TableCell v-if="isVisible('time')" class="text-no-wrap text-medium-emphasis">{{
                  formatDateTime(m.eventTime)
                }}</TableCell>
                <TableCell v-if="isVisible('messageId')" class="text-no-wrap">
                  <span class="font-mono text-caption" :title="m.messageId">{{
                    shortId(m.messageId)
                  }}</span>
                  <v-btn
                    icon="mdi-content-copy"
                    variant="text"
                    size="x-small"
                    class="copy-btn ml-1"
                    aria-label="Copy message ID"
                    title="Copy message ID"
                    @click.stop="copyMessageId(m)"
                  />
                </TableCell>
                <TableCell v-if="isVisible('mailclass')">{{ m.mailclass }}</TableCell>
                <TableCell v-if="isVisible('from')" style="max-width: 200px">
                  <span class="d-block text-truncate" :title="m.fromHeader">{{
                    fromDisplay(m.fromHeader) || '—'
                  }}</span>
                </TableCell>
                <TableCell v-if="isVisible('sender')" style="max-width: 220px">
                  <span class="d-block text-truncate text-medium-emphasis" :title="m.sender">{{
                    m.sender
                  }}</span>
                </TableCell>
                <TableCell v-if="isVisible('recipient')">{{ m.recipient }}</TableCell>
                <TableCell v-if="isVisible('recipientDomain')" class="text-medium-emphasis">{{
                  m.recipientDomain || '—'
                }}</TableCell>
                <TableCell v-if="isVisible('vmta')" class="font-mono text-caption">{{
                  m.egressSource || m.vmtaId || '—'
                }}</TableCell>
                <TableCell v-if="isVisible('node')" class="font-mono text-caption">{{
                  m.node || '—'
                }}</TableCell>
                <TableCell v-if="isVisible('status')"><StatusBadge :status="m.status" /></TableCell>
                <TableCell v-if="isVisible('type')" class="text-no-wrap text-caption text-medium-emphasis">{{
                  m.recordType || '—'
                }}</TableCell>
                <TableCell v-if="isVisible('class')" class="text-no-wrap text-caption">{{
                  m.classification || '—'
                }}</TableCell>
                <TableCell v-if="isVisible('reason')" style="max-width: 448px">
                  <span
                    v-if="m.smtpStatus || m.diagnostic"
                    class="font-mono text-caption text-medium-emphasis"
                    :title="`${m.smtpStatus} ${m.diagnostic}`.trim()"
                  >
                    <span v-if="m.smtpStatus" class="font-weight-bold">{{ m.smtpStatus }}</span>
                    <span class="d-block text-truncate">{{ m.diagnostic }}</span>
                  </span>
                  <span v-else class="text-medium-emphasis">—</span>
                </TableCell>
              </TableRow>
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

    <!-- Inspection drawer: keeps the table visible for cross-row comparison
         (see docs/vuetify-migration-plan.md — detail-view framework). -->
    <!-- disable-route-watcher: selecting a record rewrites the ?record= query
         (a route change), which would otherwise instantly close the drawer. -->
    <v-navigation-drawer
      v-model="detailOpen"
      location="right"
      temporary
      disable-route-watcher
      width="480"
      role="dialog"
      aria-label="Mail record detail"
    >
      <template #prepend>
        <div class="d-flex align-center justify-space-between px-4 py-2 border-b">
          <span class="text-caption text-uppercase text-medium-emphasis">Record detail</span>
          <v-btn
            icon="mdi-close"
            variant="text"
            size="small"
            aria-label="Close detail"
            @click="select(null)"
          />
        </div>
      </template>
      <MailRecordDetail
        v-if="selected"
        :record="selected"
        :related="related"
        @select="select($event)"
      />
    </v-navigation-drawer>
  </div>
</template>

<style scoped>
.row-clickable {
  cursor: pointer;
}
.row-selected {
  background: rgba(var(--v-theme-primary), 0.08);
}
/* The copy affordance stays quiet until the row is hovered. */
.copy-btn {
  opacity: 0;
  transition: opacity 0.15s ease;
}
.row-clickable:hover .copy-btn,
.copy-btn:focus-visible {
  opacity: 1;
}
</style>
