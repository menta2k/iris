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
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { clusterService, outboundConfigService } from '@/services'
import { ApiError } from '@/services/http'
import type { Listener, MTANode, VMTA } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<VMTA>({
  loader: () => outboundConfigService.listVmtas(),
})
const { toast } = useToast()

const VMTA_STATUSES = ['active', 'disabled', 'draining']
const vmtaStatusItems = VMTA_STATUSES.map((s) => ({ title: s, value: s }))

// Listeners a VMTA can attach to, loaded when the dialog opens so the Listener
// field is a dropdown (and IP/EHLO can be previewed in the form).
const availableListeners = ref<Listener[]>([])

// Cluster nodes for the optional node-ownership selector ('' = local node).
const availableNodes = ref<MTANode[]>([])
const nodeItems = computed(() => [
  { title: '\u2014 Local node \u2014', value: '' },
  ...availableNodes.value.map((n) => ({
    title: n.proxyHost ? `${n.name} (proxy ${n.proxyHost}:${n.proxyPort})` : n.name,
    value: n.id,
  })),
])

async function loadNodes() {
  try {
    const res = await clusterService.listNodes()
    availableNodes.value = (res.items ?? []).filter((n) => n.status !== 'disabled')
  } catch {
    availableNodes.value = []
  }
}

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref({
  name: '',
  ip_address: '',
  ehlo_name: '',
  listener_id: '',
  max_connections: 0,
  tls_mode: '',
  status: 'active',
  notes: '',
  node_id: '',
})

// Per-VMTA outbound TLS override. '' = follow the per-domain TLS Policy / default.
const TLS_ITEMS = [
  { title: 'Default (per-domain policy / opportunistic)', value: '' },
  { title: 'Required — STARTTLS + valid cert', value: 'required' },
  { title: 'Required insecure — STARTTLS, skip cert', value: 'required_insecure' },
  { title: 'Opportunistic insecure — try TLS, fall back to cleartext', value: 'opportunistic_insecure' },
  { title: 'Disabled — never TLS (cleartext)', value: 'disabled' },
]

const isEdit = computed(() => mode.value === 'edit')

const listenerOptions = computed(() =>
  availableListeners.value.map((l) => ({
    id: l.id,
    label: `${l.name} (${l.ipAddress}:${l.port})`,
  })),
)

// v-select items for the optional listener association ('' = none).
const listenerItems = computed(() => [
  { title: '— None —', value: '' },
  ...listenerOptions.value.map((o) => ({ title: o.label, value: o.id })),
])

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
}

// Convenience: when a listener is picked, copy its IP/EHLO into the (editable)
// fields if they are still empty. The VMTA OWNS these values now — the listener
// is just an optional source to copy from — so we never clobber operator input.
function fillFromListener() {
  const l = selectedListener.value
  if (!l) return
  if (!form.value.ip_address) form.value.ip_address = l.ipAddress
  if (!form.value.ehlo_name) form.value.ehlo_name = l.hostname
}

async function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = {
    name: '',
    ip_address: '',
    ehlo_name: '',
    listener_id: '',
    max_connections: 0,
    tls_mode: '',
    status: 'active',
    notes: '',
    node_id: '',
  }
  dialogOpen.value = true
  await Promise.all([loadListeners(), loadNodes()])
}

async function openEdit(v: VMTA) {
  mode.value = 'edit'
  editId.value = v.id
  form.value = {
    name: v.name,
    ip_address: v.ipAddress ?? '',
    ehlo_name: v.ehloName ?? '',
    listener_id: v.listenerId ?? '',
    max_connections: v.maxConnections ?? 0,
    tls_mode: v.tlsMode ?? '',
    status: (v.status || 'active').toLowerCase(),
    notes: v.notes ?? '',
    node_id: v.nodeId ?? '',
  }
  dialogOpen.value = true
  await Promise.all([loadListeners(), loadNodes()])
}

async function submit() {
  if (!form.value.name || !form.value.ip_address || !form.value.ehlo_name) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await outboundConfigService.updateVmta(editId.value, {
        name: form.value.name,
        ip_address: form.value.ip_address,
        ehlo_name: form.value.ehlo_name,
        listener_id: form.value.listener_id,
        max_connections: Number(form.value.max_connections),
        status: form.value.status,
        notes: form.value.notes,
        tls_mode: form.value.tls_mode,
        node_id: form.value.node_id,
      })
      toast({ title: 'VMTA updated', description: form.value.name, variant: 'success' })
    } else {
      await outboundConfigService.createVmta({
        name: form.value.name,
        ip_address: form.value.ip_address,
        ehlo_name: form.value.ehlo_name,
        listener_id: form.value.listener_id,
        max_connections: Number(form.value.max_connections),
        tls_mode: form.value.tls_mode,
        node_id: form.value.node_id,
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
                <TableHead>Node</TableHead>
                <TableHead>Max Conn</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="v in items" :key="v.id">
                <TableCell class="font-medium">{{ v.name }}</TableCell>
                <TableCell>{{ v.listenerName || '—' }}</TableCell>
                <TableCell class="font-mono text-caption">{{ v.ipAddress }}</TableCell>
                <TableCell>{{ v.ehloName }}</TableCell>
                <TableCell>{{ v.nodeName || 'local' }}</TableCell>
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
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="vmta-name">Name</Label>
          <Input id="vmta-name" v-model="form.name" placeholder="vmta-east-1" />
        </div>
        <v-row dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="vmta-ip">IP Address</Label>
            <Input id="vmta-ip" v-model="form.ip_address" placeholder="203.0.113.10" />
          </v-col>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="vmta-ehlo">EHLO Name</Label>
            <Input id="vmta-ehlo" v-model="form.ehlo_name" placeholder="mta1.example.com" />
          </v-col>
        </v-row>
        <div class="d-flex flex-column ga-1">
          <Label for="vmta-listener">Listener (optional)</Label>
          <v-select
            id="vmta-listener"
            data-testid="vmta-listener"
            :model-value="form.listener_id"
            :items="listenerItems"
            variant="outlined"
            density="compact"
            hide-details
            @update:model-value="
              (v: string) => {
                form.listener_id = v
                fillFromListener()
              }
            "
          />
          <p class="text-caption text-medium-emphasis">
            Optional association (e.g. the listener that receives this IP's bounces). Selecting one
            pre-fills the IP/EHLO above if they're empty — the VMTA owns those values.
          </p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="vmta-node">Cluster node</Label>
          <v-select
            id="vmta-node"
            v-model="form.node_id"
            data-testid="vmta-node"
            :items="nodeItems"
            variant="outlined"
            density="compact"
            hide-details
          />
          <p class="text-caption text-medium-emphasis">
            The node this VMTA's IP is physically bound on. Mail routed to this VMTA from other
            nodes is delivered through that node's kumo-proxy, so it always egresses from this IP.
          </p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="vmta-max-conn">Max Connections</Label>
          <Input
            id="vmta-max-conn"
            v-model.number="form.max_connections"
            type="number"
            placeholder="0"
          />
          <p class="text-caption text-medium-emphasis">0 = unlimited.</p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="vmta-tls">Outbound TLS</Label>
          <v-select
            id="vmta-tls"
            v-model="form.tls_mode"
            :items="TLS_ITEMS"
            variant="outlined"
            density="compact"
            hide-details
          />
          <p class="text-caption text-medium-emphasis">
            Forces STARTTLS (or relaxes it) for any mail sent from this VMTA. A per-domain
            <strong>TLS Policy</strong> still takes precedence when both apply.
          </p>
        </div>
        <template v-if="isEdit">
          <div class="d-flex flex-column ga-1">
            <Label for="vmta-status">Status</Label>
            <v-select
              id="vmta-status"
              v-model="form.status"
              :items="vmtaStatusItems"
              variant="outlined"
              density="compact"
              hide-details
            />
          </div>
          <div class="d-flex flex-column ga-1">
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
