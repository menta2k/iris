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
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { usePagedList } from '@/composables/usePagedList'
import { useConfigStore } from '@/stores/config'
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

const filters = ref<MailRecordFilters>({
  mailclass: '',
  sender: '',
  from: '',
  recipient: '',
  vmta_id: '',
  record_type: '',
})

// The loader reads filters at call time, so reload() applies the current values.
const {
  items,
  loading,
  error,
  notImplemented,
  pageSize,
  pageNumber,
  hasPrev,
  hasNext,
  reload,
  nextPage,
  prevPage,
  setPageSize,
} = usePagedList<MailRecord>({
  loader: (page) => mailOperationsService.listMailRecords({ ...filters.value }, page),
})

function resetFilters() {
  filters.value = { mailclass: '', sender: '', from: '', recipient: '', vmta_id: '', record_type: '' }
  reload()
}

// ---- Column visibility & density (persisted per-table in the config store) ----

const TABLE_ID = 'mail-logs'

const ALL_COLUMNS = [
  { key: 'time', title: 'Time' },
  { key: 'messageId', title: 'Message ID' },
  { key: 'mailclass', title: 'Mailclass' },
  { key: 'from', title: 'From' },
  { key: 'sender', title: 'Sender (envelope)' },
  { key: 'recipient', title: 'Recipient' },
  { key: 'vmta', title: 'VMTA' },
  { key: 'status', title: 'Status' },
  { key: 'type', title: 'Type' },
  { key: 'class', title: 'Class' },
  { key: 'reason', title: 'Reason' },
] as const

type ColumnKey = (typeof ALL_COLUMNS)[number]['key']

const config = useConfigStore()

const hiddenColumns = computed<string[]>(() => config.tablePrefs[TABLE_ID]?.hidden ?? [])
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
onBeforeUnmount(() => window.removeEventListener('keydown', onEsc))
</script>

<template>
  <div>
    <PageHeader title="Mail Logs" description="Searchable record of message-level delivery events." />

    <Card class="mb-4">
      <CardContent class="pa-4">
        <form @submit.prevent="reload">
          <v-row dense align="end">
            <v-col cols="12" sm="6" md="2">
              <Label for="f-mailclass">Mailclass</Label>
              <Input id="f-mailclass" v-model="filters.mailclass" placeholder="marketing" />
            </v-col>
            <v-col cols="12" sm="6" md="2">
              <Label for="f-from">From</Label>
              <Input id="f-from" v-model="filters.from" placeholder="sentry@infra.example.com" />
            </v-col>
            <v-col cols="12" sm="6" md="2">
              <Label for="f-sender">Sender (envelope)</Label>
              <Input id="f-sender" v-model="filters.sender" placeholder="news@example.com" />
            </v-col>
            <v-col cols="12" sm="6" md="2">
              <Label for="f-recipient">Recipient</Label>
              <Input id="f-recipient" v-model="filters.recipient" placeholder="user@gmail.com" />
            </v-col>
            <v-col cols="12" sm="6" md="2">
              <Label for="f-vmta">VMTA</Label>
              <Input id="f-vmta" v-model="filters.vmta_id" placeholder="vmta-1" />
            </v-col>
            <v-col cols="12" sm="6" md="2">
              <Label for="f-type">Type</Label>
              <v-select
                id="f-type"
                :model-value="filters.record_type"
                :items="TYPE_ITEMS"
                data-testid="mail-record-type"
                variant="outlined"
                density="compact"
                hide-details
                @update:model-value="filters.record_type = $event"
              />
            </v-col>
            <v-col cols="12" class="d-flex align-center ga-2">
              <Button type="submit" data-testid="apply-filters">Filter</Button>
              <Button type="button" variant="outline" @click="resetFilters">Reset</Button>
              <v-spacer />
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
                  <v-list-item
                    v-for="col in ALL_COLUMNS"
                    :key="col.key"
                    @click="toggleColumn(col.key)"
                  >
                    <template #prepend>
                      <v-checkbox-btn :model-value="isVisible(col.key)" density="compact" />
                    </template>
                    <v-list-item-title class="text-body-2">{{ col.title }}</v-list-item-title>
                  </v-list-item>
                </v-list>
              </v-menu>
            </v-col>
          </v-row>
        </form>
      </CardContent>
    </Card>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No mail records match the selected filters."
    >
      <Card>
        <CardContent class="pa-0">
          <Table :density="density" fixed-header height="calc(100vh - 420px)">
            <TableHeader>
              <TableRow>
                <TableHead v-if="isVisible('time')">Time</TableHead>
                <TableHead v-if="isVisible('messageId')">Message ID</TableHead>
                <TableHead v-if="isVisible('mailclass')">Mailclass</TableHead>
                <TableHead v-if="isVisible('from')">From</TableHead>
                <TableHead v-if="isVisible('sender')">Sender (envelope)</TableHead>
                <TableHead v-if="isVisible('recipient')">Recipient</TableHead>
                <TableHead v-if="isVisible('vmta')">VMTA</TableHead>
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
                <TableCell v-if="isVisible('messageId')" class="font-mono text-caption">{{
                  m.messageId
                }}</TableCell>
                <TableCell v-if="isVisible('mailclass')">{{ m.mailclass }}</TableCell>
                <TableCell v-if="isVisible('from')">{{ m.fromHeader || '—' }}</TableCell>
                <TableCell v-if="isVisible('sender')" style="max-width: 220px">
                  <span class="d-block text-truncate text-medium-emphasis" :title="m.sender">{{
                    m.sender
                  }}</span>
                </TableCell>
                <TableCell v-if="isVisible('recipient')">{{ m.recipient }}</TableCell>
                <TableCell v-if="isVisible('vmta')" class="font-mono text-caption">{{
                  m.egressSource || m.vmtaId || '—'
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
        </CardContent>
      </Card>
    </DataState>

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
</style>
