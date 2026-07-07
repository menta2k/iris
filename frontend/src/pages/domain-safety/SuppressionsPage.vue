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
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { usePagedList } from '@/composables/usePagedList'
import { useToast } from '@/composables/useToast'
import { domainSafetyService } from '@/services'
import { ApiError } from '@/services/http'
import type { DsnMessage, Suppression } from '@/types'

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
  loader: (page) => domainSafetyService.listSuppressions(page, search.value),
})
const { toast } = useToast()

// Case-insensitive substring search on the suppressed value, debounced so
// typing doesn't fire a request per keystroke. Resets to the first page.
const search = ref('')
let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => reload(), 300)
})
onBeforeUnmount(() => clearTimeout(searchTimer))

const SUPPRESSION_STATUSES = ['active', 'disabled', 'expired']
const SUPPRESSION_STATUS_ITEMS = SUPPRESSION_STATUSES.map((s) => ({ title: s, value: s }))
const SUPPRESSION_TYPE_ITEMS = [
  { title: 'email', value: 'email' },
  { title: 'domain', value: 'domain' },
]

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

function formatDate(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString()
}

// DSN message viewer (for dsn-sourced suppressions).
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
</script>

<template>
  <div>
    <PageHeader title="Suppressions" description="Recipients and domains suppressed from future delivery.">
      <template #actions>
        <Input
          v-model="search"
          placeholder="Search address or domain…"
          clearable
          prepend-inner-icon="mdi-magnify"
          style="min-width: 260px"
          data-testid="search-suppression"
        />
        <Button data-testid="create-suppression" @click="openCreate">Add Suppression</Button>
      </template>
    </PageHeader>
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No suppressions on record."
    >
      <Card>
        <CardContent class="pa-0">
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
              <TableRow v-for="s in items" :key="s.id">
                <TableCell><Badge variant="outline">{{ s.type }}</Badge></TableCell>
                <TableCell class="font-weight-medium">{{ s.value }}</TableCell>
                <TableCell>
                  <Badge v-if="s.mailclass" variant="secondary">{{ s.mailclass }}</Badge>
                  <span v-else class="text-medium-emphasis">—</span>
                </TableCell>
                <TableCell><Badge variant="destructive">{{ s.reason }}</Badge></TableCell>
                <TableCell class="text-medium-emphasis">{{ s.source }}</TableCell>
                <TableCell class="text-caption text-no-wrap">{{ formatDate(s.createdAt) }}</TableCell>
                <TableCell class="text-caption text-no-wrap">
                  <span v-if="s.expiresAt">{{ formatDate(s.expiresAt) }}</span>
                  <span v-else class="text-medium-emphasis">Never</span>
                </TableCell>
                <TableCell><StatusBadge :status="s.status" /></TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-1">
                    <Button
                      v-if="s.source === 'dsn'"
                      variant="outline"
                      size="sm"
                      :data-testid="`view-dsn-${s.id}`"
                      @click="viewDsn(s)"
                    >
                      View message
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      :data-testid="`edit-suppression-${s.id}`"
                      @click="openEdit(s)"
                    >
                      Edit
                    </Button>
                  </div>
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
