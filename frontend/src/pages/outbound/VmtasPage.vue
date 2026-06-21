<script setup lang="ts">
import { computed, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
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
import { Select } from '@/components/ui/select'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { outboundConfigService } from '@/services'
import { ApiError } from '@/services/http'
import type { Listener, VMTA } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<VMTA>({
  loader: () => outboundConfigService.listVmtas(),
})
const { toast } = useToast()

const VMTA_STATUSES = ['active', 'disabled', 'draining']

// Listeners a VMTA can attach to, loaded when the dialog opens so the Listener
// field is a dropdown (and IP/EHLO can be previewed in the form).
const availableListeners = ref<Listener[]>([])

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref({
  name: '',
  listener_id: '',
  max_connections: 0,
  status: 'active',
  notes: '',
})

const isEdit = computed(() => mode.value === 'edit')

const listenerOptions = computed(() =>
  availableListeners.value.map((l) => ({
    id: l.id,
    label: `${l.name} (${l.ipAddress}:${l.port})`,
  })),
)

// The listener currently selected in the form, used to preview its resolved
// IP/EHLO read-only fields.
const selectedListener = computed(() =>
  availableListeners.value.find((l) => l.id === form.value.listener_id),
)

async function loadListeners() {
  try {
    const res = await outboundConfigService.listListeners()
    availableListeners.value = res.items ?? []
  } catch {
    availableListeners.value = []
  }
  ensureListenerSelected()
}

// Default the Listener dropdown to the first option when nothing is chosen yet,
// so a freshly opened create dialog isn't stuck with a disabled Create button.
function ensureListenerSelected() {
  if (!form.value.listener_id && listenerOptions.value.length) {
    form.value.listener_id = listenerOptions.value[0].id
  }
}

async function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = { name: '', listener_id: '', max_connections: 0, status: 'active', notes: '' }
  dialogOpen.value = true
  await loadListeners()
}

async function openEdit(v: VMTA) {
  mode.value = 'edit'
  editId.value = v.id
  form.value = {
    name: v.name,
    listener_id: v.listenerId,
    max_connections: v.maxConnections ?? 0,
    status: (v.status || 'active').toLowerCase(),
    notes: v.notes ?? '',
  }
  dialogOpen.value = true
  await loadListeners()
}

async function submit() {
  if (!form.value.name || !form.value.listener_id) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await outboundConfigService.updateVmta(editId.value, {
        name: form.value.name,
        listener_id: form.value.listener_id,
        max_connections: Number(form.value.max_connections),
        status: form.value.status,
        notes: form.value.notes,
      })
      toast({ title: 'VMTA updated', description: form.value.name, variant: 'success' })
    } else {
      await outboundConfigService.createVmta({
        name: form.value.name,
        listener_id: form.value.listener_id,
        max_connections: Number(form.value.max_connections),
      })
      toast({ title: 'VMTA created', description: form.value.name, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save VMTA.'
    toast({
      title: isEdit.value ? 'Update failed' : 'Create failed',
      description: msg,
      variant: 'destructive',
    })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="VMTAs" description="Virtual MTAs used for outbound delivery.">
      <template #actions>
        <Button data-testid="create-vmta" @click="openCreate">New VMTA</Button>
      </template>
    </PageHeader>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No VMTAs configured yet."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Listener</TableHead>
                <TableHead>IP Address</TableHead>
                <TableHead>EHLO Name</TableHead>
                <TableHead>Max Conn</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="v in items" :key="v.id">
                <TableCell class="font-medium">{{ v.name }}</TableCell>
                <TableCell>{{ v.listenerName || '—' }}</TableCell>
                <TableCell class="font-mono text-xs">{{ v.ipAddress }}</TableCell>
                <TableCell>{{ v.ehloName }}</TableCell>
                <TableCell class="tabular-nums">
                  {{ v.maxConnections === 0 ? 'unlimited' : v.maxConnections }}
                </TableCell>
                <TableCell><StatusBadge :status="v.status" /></TableCell>
                <TableCell class="text-right">
                  <Button
                    variant="outline"
                    size="sm"
                    :data-testid="`edit-vmta-${v.id}`"
                    @click="openEdit(v)"
                  >
                    Edit
                  </Button>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>

    <Dialog v-model:open="dialogOpen">
      <DialogHeader>
        <DialogTitle>{{ isEdit ? 'Edit VMTA' : 'Create VMTA' }}</DialogTitle>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="space-y-1.5">
          <Label for="vmta-name">Name</Label>
          <Input id="vmta-name" v-model="form.name" placeholder="vmta-east-1" />
        </div>
        <div class="space-y-1.5">
          <Label for="vmta-listener">Listener</Label>
          <Select id="vmta-listener" v-model="form.listener_id" data-testid="vmta-listener">
            <option value="" disabled>
              {{
                listenerOptions.length
                  ? 'Select a listener…'
                  : 'No listeners — create one first'
              }}
            </option>
            <option v-for="o in listenerOptions" :key="o.id" :value="o.id">{{ o.label }}</option>
          </Select>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="vmta-ip">IP Address (from listener)</Label>
            <Input
              id="vmta-ip"
              :model-value="selectedListener?.ipAddress ?? ''"
              disabled
              placeholder="—"
            />
          </div>
          <div class="space-y-1.5">
            <Label for="vmta-ehlo">EHLO Name (from listener)</Label>
            <Input
              id="vmta-ehlo"
              :model-value="selectedListener?.hostname ?? ''"
              disabled
              placeholder="—"
            />
          </div>
        </div>
        <div class="space-y-1.5">
          <Label for="vmta-max-conn">Max Connections</Label>
          <Input
            id="vmta-max-conn"
            v-model.number="form.max_connections"
            type="number"
            placeholder="0"
          />
          <p class="text-xs text-muted-foreground">0 = unlimited.</p>
        </div>
        <template v-if="isEdit">
          <div class="space-y-1.5">
            <Label for="vmta-status">Status</Label>
            <Select id="vmta-status" v-model="form.status">
              <option v-for="s in VMTA_STATUSES" :key="s" :value="s">{{ s }}</option>
            </Select>
          </div>
          <div class="space-y-1.5">
            <Label for="vmta-notes">Notes</Label>
            <Input id="vmta-notes" v-model="form.notes" placeholder="Optional operator notes" />
          </div>
        </template>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !form.name || !form.listener_id">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Create' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
