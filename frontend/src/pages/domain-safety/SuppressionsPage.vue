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
  TableEmpty,
} from '@/components/ui/table'
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { usePagedList } from '@/composables/usePagedList'
import { useToast } from '@/composables/useToast'
import { domainSafetyService } from '@/services'
import { ApiError } from '@/services/http'
import type { DsnMessage, Suppression, SuppressionFilters } from '@/types'

const TYPE_ITEMS = [
  { title: 'All types', value: '' },
  { title: 'Email', value: 'email' },
  { title: 'Domain', value: 'domain' },
]
const STATUS_ITEMS = [
  { title: 'All statuses', value: '' },
  { title: 'Active', value: 'active' },
  { title: 'Disabled', value: 'disabled' },
  { title: 'Expired', value: 'expired' },
]
const SOURCE_ITEMS = [
  { title: 'All sources', value: '' },
  { title: 'Manual', value: 'manual' },
  { title: 'Bounce', value: 'bounce' },
  { title: 'Feedback (FBL)', value: 'feedback' },
  { title: 'DSN (async bounce)', value: 'dsn' },
]

const filters = ref<SuppressionFilters>({
  search: '',
  mailclass: '',
  type: '',
  status: '',
  source: '',
})

// v-text-field `clearable` writes null; the loader reads filters at call time.
function buildFilters(): SuppressionFilters {
  return Object.fromEntries(
    Object.entries(filters.value).map(([k, v]) => [k, v ?? '']),
  ) as SuppressionFilters
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
  reload,
  nextPage,
  prevPage,
  setPageSize,
} = usePagedList<Suppression>({
  loader: (page) => domainSafetyService.listSuppressions(page, buildFilters()),
})
const { toast } = useToast()

// Text fields apply as you type (debounced); selects apply immediately.
let debounceTimer: ReturnType<typeof setTimeout> | undefined
watch(
  () => [filters.value.search, filters.value.mailclass],
  () => {
    clearTimeout(debounceTimer)
    debounceTimer = setTimeout(reload, 300)
  },
)
watch(() => [filters.value.type, filters.value.status, filters.value.source], () => reload())
onBeforeUnmount(() => clearTimeout(debounceTimer))

const hasActiveFilters = computed(() =>
  Object.values(filters.value).some((v) => (v ?? '') !== ''),
)

function resetFilters() {
  clearTimeout(debounceTimer)
  filters.value = { search: '', mailclass: '', type: '', status: '', source: '' }
  reload()
}

const SUPPRESSION_STATUSES = ['active', 'disabled', 'expired']
const SUPPRESSION_STATUS_ITEMS = SUPPRESSION_STATUSES.map((s) => ({ title: s, value: s }))
const SUPPRESSION_TYPE_ITEMS = [
  { title: 'email', value: 'email' },
  { title: 'domain', value: 'domain' },
]

function formatDate(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString()
}

// Compact relative time ("in 21d", "3h ago") so expiry reads at a glance.
function relative(iso?: string): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ''
  const diff = t - Date.now()
  const abs = Math.abs(diff)
  const day = 86_400_000
  const hour = 3_600_000
  const span =
    abs >= day ? `${Math.round(abs / day)}d` : abs >= hour ? `${Math.round(abs / hour)}h` : '<1h'
  return diff > 0 ? `in ${span}` : `${span} ago`
}

// ---- Quick enable/disable straight from the row ----

const togglingId = ref<string | null>(null)

async function toggleStatus(s: Suppression) {
  const next = (s.status || '').toLowerCase() === 'active' ? 'disabled' : 'active'
  togglingId.value = s.id
  try {
    await domainSafetyService.updateSuppression(s.id, { reason: s.reason, status: next })
    toast({
      title: next === 'active' ? 'Suppression enabled' : 'Suppression disabled',
      description: s.value,
      variant: 'success',
    })
    reload()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to update suppression.'
    toast({ title: 'Update failed', description: msg, variant: 'destructive' })
  } finally {
    togglingId.value = null
  }
}

// ---- CSV export of the current page ----

function exportCsv() {
  const esc = (v: string) => `"${v.replace(/"/g, '""')}"`
  const header = ['Type', 'Value', 'Mailclass', 'Reason', 'Source', 'Suppressed', 'Expires', 'Status']
  const lines = [
    header.map(esc).join(','),
    ...items.value.map((s) =>
      [
        s.type ?? '',
        s.value ?? '',
        s.mailclass ?? '',
        s.reason ?? '',
        s.source ?? '',
        s.createdAt ?? '',
        s.expiresAt ?? '',
        s.status ?? '',
      ].map(esc).join(','),
    ),
  ]
  const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `suppressions-page-${pageNumber.value}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

// ---- Create/edit dialog ----

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref<{
  type: 'email' | 'domain'
  value: string
  reason: string
  status: string
}>({
  type: 'email',
  value: '',
  reason: '',
  status: 'active',
})

const isEdit = computed(() => mode.value === 'edit')

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = { type: 'email', value: '', reason: '', status: 'active' }
  dialogOpen.value = true
}

function openEdit(s: Suppression) {
  mode.value = 'edit'
  editId.value = s.id
  form.value = {
    type: (s.type as 'email' | 'domain') || 'email',
    value: s.value,
    reason: s.reason,
    status: (s.status || 'active').toLowerCase(),
  }
  dialogOpen.value = true
}

async function submit() {
  if (!isEdit.value && !form.value.value) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await domainSafetyService.updateSuppression(editId.value, {
        reason: form.value.reason,
        status: form.value.status,
      })
      toast({ title: 'Suppression updated', description: form.value.value, variant: 'success' })
    } else {
      await domainSafetyService.createSuppression({
        type: form.value.type,
        value: form.value.value,
        reason: form.value.reason,
      })
      toast({ title: 'Suppression added', description: form.value.value, variant: 'success' })
    }
    dialogOpen.value = false
    reload()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save suppression.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

// ---- DSN message viewer (for dsn-sourced suppressions) ----

const dsnDialogOpen = ref(false)
const dsnLoading = ref(false)
const dsnError = ref<string | null>(null)
const dsnValue = ref('')
const dsnMessages = ref<DsnMessage[]>([])

async function viewDsn(s: Suppression) {
  dsnValue.value = s.value
  dsnMessages.value = []
  dsnError.value = null
  dsnDialogOpen.value = true
  dsnLoading.value = true
  try {
    const res = await domainSafetyService.listSuppressionDsnMessages(s.id)
    dsnMessages.value = res.items ?? []
  } catch (err) {
    dsnError.value = err instanceof ApiError ? err.message : 'Failed to load DSN message.'
  } finally {
    dsnLoading.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="Suppressions" description="Recipients and domains suppressed from future delivery.">
      <template #actions>
        <Button data-testid="create-suppression" @click="openCreate">Add Suppression</Button>
      </template>
    </PageHeader>

    <Card class="mb-4">
      <CardContent class="pa-4">
        <v-row dense>
          <v-col cols="12" sm="6" md="3">
            <v-text-field
              v-model="filters.search"
              label="Search"
              placeholder="Address or domain"
              prepend-inner-icon="mdi-magnify"
              variant="outlined"
              density="compact"
              hide-details
              clearable
              data-testid="search-suppression"
            />
          </v-col>
          <v-col cols="12" sm="6" md="2">
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
          <v-col cols="12" sm="6" md="2">
            <v-select
              v-model="filters.type"
              :items="TYPE_ITEMS"
              data-testid="suppression-type-filter"
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
              data-testid="suppression-status-filter"
              label="Status"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="2">
            <v-select
              v-model="filters.source"
              :items="SOURCE_ITEMS"
              data-testid="suppression-source-filter"
              label="Source"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="1" class="d-flex align-center">
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
        <span class="text-subtitle-1 font-weight-bold mr-1">Entries</span>
        <span class="text-caption text-medium-emphasis">
          page {{ pageNumber }}<template v-if="items.length"> · {{ items.length }} rows</template>
        </span>
        <v-spacer />
        <v-btn
          icon="mdi-refresh"
          variant="text"
          size="small"
          :loading="loading"
          aria-label="Refresh"
          title="Refresh"
          @click="reload"
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
        <!-- Keep the table mounted during as-you-type filtering; the full
             spinner only covers the first load. -->
        <DataState
          :loading="loading && items.length === 0"
          :error="error"
          :not-implemented="notImplemented"
          :empty="items.length === 0"
          empty-message="No suppressions match the selected filters."
        >
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Type</TableHead>
                <TableHead>Value</TableHead>
                <TableHead>Mailclass</TableHead>
                <TableHead>Reason</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Suppressed</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableEmpty v-if="items.length === 0" :colspan="9" message="No suppressions on record." />
              <TableRow v-for="s in items" :key="s.id">
                <TableCell><Badge variant="outline">{{ s.type }}</Badge></TableCell>
                <TableCell class="font-weight-medium">{{ s.value }}</TableCell>
                <TableCell>
                  <Badge v-if="s.mailclass" variant="secondary">{{ s.mailclass }}</Badge>
                  <span v-else class="text-medium-emphasis">—</span>
                </TableCell>
                <TableCell style="max-width: 260px">
                  <span class="d-block text-truncate text-medium-emphasis" :title="s.reason">
                    {{ s.reason || '—' }}
                  </span>
                </TableCell>
                <TableCell class="text-medium-emphasis">{{ s.source }}</TableCell>
                <TableCell class="text-caption text-no-wrap">{{ formatDate(s.createdAt) }}</TableCell>
                <TableCell class="text-caption text-no-wrap">
                  <template v-if="s.expiresAt">
                    {{ formatDate(s.expiresAt) }}
                    <span class="d-block text-medium-emphasis">{{ relative(s.expiresAt) }}</span>
                  </template>
                  <span v-else class="text-medium-emphasis">Never</span>
                </TableCell>
                <TableCell><StatusBadge :status="s.status" /></TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-1">
                    <v-btn
                      v-if="s.source === 'dsn'"
                      icon="mdi-email-search-outline"
                      variant="text"
                      size="small"
                      aria-label="View DSN message"
                      title="View DSN message"
                      :data-testid="`view-dsn-${s.id}`"
                      @click="viewDsn(s)"
                    />
                    <v-btn
                      :icon="(s.status || '').toLowerCase() === 'active' ? 'mdi-pause-circle-outline' : 'mdi-play-circle-outline'"
                      variant="text"
                      size="small"
                      :color="(s.status || '').toLowerCase() === 'active' ? 'warning' : 'success'"
                      :loading="togglingId === s.id"
                      :aria-label="(s.status || '').toLowerCase() === 'active' ? 'Disable suppression' : 'Enable suppression'"
                      :title="(s.status || '').toLowerCase() === 'active' ? 'Disable — allow delivery again' : 'Enable — block delivery'"
                      :data-testid="`toggle-suppression-${s.id}`"
                      @click="toggleStatus(s)"
                    />
                    <v-btn
                      icon="mdi-pencil-outline"
                      variant="text"
                      size="small"
                      aria-label="Edit suppression"
                      title="Edit suppression"
                      :data-testid="`edit-suppression-${s.id}`"
                      @click="openEdit(s)"
                    />
                  </div>
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

    <Dialog v-model:open="dialogOpen">
      <DialogHeader>
        <DialogTitle>{{ isEdit ? 'Edit Suppression' : 'Add Suppression' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="supp-type">Type</Label>
          <v-select
            id="supp-type"
            v-model="form.type"
            :items="SUPPRESSION_TYPE_ITEMS"
            :disabled="isEdit"
            variant="outlined"
            density="compact"
            hide-details
          />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="supp-value">Value</Label>
          <Input
            id="supp-value"
            v-model="form.value"
            :disabled="isEdit"
            :placeholder="form.type === 'domain' ? 'example.com' : 'user@example.com'"
          />
          <p v-if="isEdit" class="text-caption text-medium-emphasis">Type and value are immutable.</p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="supp-reason">Reason</Label>
          <Input id="supp-reason" v-model="form.reason" placeholder="hard_bounce" />
        </div>
        <div v-if="isEdit" class="d-flex flex-column ga-1">
          <Label for="supp-status">Status</Label>
          <v-select
            id="supp-status"
            v-model="form.status"
            :items="SUPPRESSION_STATUS_ITEMS"
            variant="outlined"
            density="compact"
            hide-details
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || (!isEdit && !form.value)">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Add Suppression' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>

    <Dialog v-model:open="dsnDialogOpen">
      <DialogHeader>
        <DialogTitle>DSN message — {{ dsnValue }}</DialogTitle>
      </DialogHeader>
      <div class="d-flex flex-column ga-3">
        <div v-if="dsnLoading" class="text-medium-emphasis text-body-2">Loading…</div>
        <div v-else-if="dsnError" class="text-error text-body-2">{{ dsnError }}</div>
        <div v-else-if="dsnMessages.length === 0" class="text-medium-emphasis text-body-2">
          No archived DSN message for this recipient. Only asynchronous bounces captured at the
          bounce domain after this feature shipped are stored.
        </div>
        <div v-for="m in dsnMessages" v-else :key="m.id" class="d-flex flex-column ga-1">
          <div class="text-caption text-medium-emphasis">
            Received {{ new Date(m.receivedAt).toLocaleString() }}
            <span v-if="m.messageId"> · message {{ m.messageId }}</span>
          </div>
          <pre class="dsn-raw">{{ m.rawMessage }}</pre>
        </div>
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" @click="dsnDialogOpen = false">Close</Button>
      </DialogFooter>
    </Dialog>
  </div>
</template>

<style scoped>
.dsn-raw {
  max-height: 50vh;
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
