<script setup lang="ts">
import { computed, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import { Card, CardContent } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { clusterService } from '@/services'
import { ApiError } from '@/services/http'
import type { MTANode, MTANodeStatus } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<MTANode>({
  loader: () => clusterService.listNodes(),
})
const { toast } = useToast()

function formatDate(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(undefined, { hour12: false })
}

function statusVariant(s: MTANodeStatus): 'success' | 'secondary' | 'warning' {
  if (s === 'active') return 'success'
  if (s === 'draining') return 'warning'
  return 'secondary'
}

// Live kumod health chip, refreshed by the backend heartbeat worker (~30s).
function healthVariant(state?: string): 'success' | 'secondary' | 'warning' | 'destructive' {
  const s = (state ?? '').toLowerCase()
  if (s === 'running') return 'success'
  if (s === 'degraded') return 'warning'
  if (s === 'unreachable') return 'destructive'
  return 'secondary'
}

/**
 * Config drift: the expected checksum is the one most recently applied across
 * the cluster (nodes report theirs on every heartbeat). A node that disagrees
 * with the majority-newest value is flagged.
 */
const expectedChecksum = computed(() => {
  const withSeen = items.value.filter((n) => n.appliedChecksum && n.lastSeenAt)
  if (!withSeen.length) return ''
  const newest = [...withSeen].sort((a, b) => (b.lastSeenAt || '').localeCompare(a.lastSeenAt || ''))[0]
  return newest.appliedChecksum
})
function hasDrift(n: MTANode): boolean {
  return !!expectedChecksum.value && !!n.appliedChecksum && n.appliedChecksum !== expectedChecksum.value
}

function shortChecksum(sum: string): string {
  return sum ? sum.slice(0, 8) : '—'
}

// ---- Create / edit dialog ----
const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)

interface FormState {
  name: string
  agentUrl: string
  proxyHost: string
  proxyPort: string
  status: MTANodeStatus
  notes: string
}
function emptyForm(): FormState {
  return { name: '', agentUrl: '', proxyHost: '', proxyPort: '', status: 'active', notes: '' }
}
const form = ref<FormState>(emptyForm())
const isEdit = computed(() => mode.value === 'edit')

const statusOptions: MTANodeStatus[] = ['active', 'draining', 'disabled']

const agentUrlError = computed(() => {
  const u = form.value.agentUrl.trim()
  if (!u) return ''
  return u.startsWith('https://') ? '' : 'Agent URL must use https (the agent channel is mTLS-only)'
})
const proxyError = computed(() => {
  const host = form.value.proxyHost.trim()
  const port = form.value.proxyPort.trim()
  if (!host && !port) return ''
  if (!host || !port) return 'Proxy host and port must be set together'
  const n = Number(port)
  if (!Number.isInteger(n) || n < 1 || n > 65535) return 'Proxy port must be 1–65535'
  return ''
})
const canSubmit = computed(
  () => !!form.value.name.trim() && !agentUrlError.value && !proxyError.value,
)

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = emptyForm()
  dialogOpen.value = true
}
function openEdit(n: MTANode) {
  mode.value = 'edit'
  editId.value = n.id
  form.value = {
    name: n.name,
    agentUrl: n.agentUrl,
    proxyHost: n.proxyHost,
    proxyPort: n.proxyPort ? String(n.proxyPort) : '',
    status: n.status,
    notes: n.notes,
  }
  dialogOpen.value = true
}

function formBody() {
  return {
    name: form.value.name.trim(),
    agentUrl: form.value.agentUrl.trim(),
    proxyHost: form.value.proxyHost.trim(),
    proxyPort: form.value.proxyPort.trim() ? Number(form.value.proxyPort) : 0,
    status: form.value.status,
    notes: form.value.notes,
  }
}

async function submit() {
  if (!canSubmit.value) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await clusterService.updateNode(editId.value, { id: editId.value, ...formBody() })
      toast({ title: 'Node updated', description: form.value.name, variant: 'success' })
    } else {
      await clusterService.createNode(formBody())
      toast({ title: 'Node registered', description: form.value.name, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save node.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

// ---- Status transitions + delete ----
async function setStatus(n: MTANode, status: MTANodeStatus) {
  try {
    await clusterService.updateNode(n.id, {
      id: n.id,
      name: n.name,
      agentUrl: n.agentUrl,
      proxyHost: n.proxyHost,
      proxyPort: n.proxyPort,
      status,
      notes: n.notes,
    })
    toast({ title: `Node ${status === 'active' ? 'activated' : status}`, description: n.name, variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to update node.'
    toast({ title: 'Update failed', description: msg, variant: 'destructive' })
  }
}

const deletingId = ref<string | null>(null)
async function remove(n: MTANode) {
  deletingId.value = n.id
  try {
    await clusterService.removeNode(n.id)
    toast({ title: 'Node removed', description: n.name, variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to delete node.'
    toast({ title: 'Delete failed', description: msg, variant: 'destructive' })
  } finally {
    deletingId.value = null
  }
}
</script>

<template>
  <div>
    <PageHeader
      title="Cluster Nodes"
      description="KumoMTA hosts managed by iris. A node without an agent URL is the co-located instance (local file/reload); remote nodes are managed through their iris-agent over mutual TLS. Config applies roll out to every non-disabled node; a checksum that differs from the newest applied one is flagged as drift."
    >
      <template #actions>
        <Button data-testid="create-node" @click="openCreate">Register Node</Button>
      </template>
    </PageHeader>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No nodes registered. Single-node deployments work without registration; register nodes to run a cluster."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Agent</TableHead>
                <TableHead>kumo-proxy</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Health</TableHead>
                <TableHead>Config</TableHead>
                <TableHead>Last seen</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="n in items" :key="n.id">
                <TableCell class="font-weight-medium">
                  {{ n.name }}
                  <div v-if="n.notes" class="text-caption text-medium-emphasis">{{ n.notes }}</div>
                </TableCell>
                <TableCell class="font-mono text-caption">
                  {{ n.agentUrl || 'local' }}
                  <div v-if="n.version" class="text-medium-emphasis">{{ n.version }}</div>
                </TableCell>
                <TableCell class="font-mono text-caption">
                  {{ n.proxyHost ? `${n.proxyHost}:${n.proxyPort}` : '—' }}
                </TableCell>
                <TableCell>
                  <Badge :variant="statusVariant(n.status)">{{ n.status }}</Badge>
                </TableCell>
                <TableCell>
                  <Badge :variant="healthVariant(n.kumoState)" :data-testid="`health-${n.id}`">
                    {{ n.kumoState || 'unknown' }}
                  </Badge>
                </TableCell>
                <TableCell>
                  <div class="d-flex align-center ga-2">
                    <span class="font-mono text-caption">{{ shortChecksum(n.appliedChecksum) }}</span>
                    <Badge v-if="hasDrift(n)" variant="destructive" :data-testid="`drift-${n.id}`">drift</Badge>
                  </div>
                </TableCell>
                <TableCell class="text-caption text-no-wrap">{{ formatDate(n.lastSeenAt) }}</TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-2">
                    <Button
                      v-if="n.status !== 'draining'"
                      variant="outline"
                      size="sm"
                      :data-testid="`drain-${n.id}`"
                      @click="setStatus(n, 'draining')"
                    >
                      Drain
                    </Button>
                    <Button
                      v-if="n.status !== 'active'"
                      variant="outline"
                      size="sm"
                      :data-testid="`activate-${n.id}`"
                      @click="setStatus(n, 'active')"
                    >
                      Activate
                    </Button>
                    <Button
                      v-if="n.status !== 'disabled'"
                      variant="outline"
                      size="sm"
                      :data-testid="`disable-${n.id}`"
                      @click="setStatus(n, 'disabled')"
                    >
                      Disable
                    </Button>
                    <Button variant="outline" size="sm" :data-testid="`edit-${n.id}`" @click="openEdit(n)">Edit</Button>
                    <Button
                      variant="outline"
                      size="sm"
                      :disabled="deletingId === n.id"
                      :data-testid="`delete-${n.id}`"
                      @click="remove(n)"
                    >
                      {{ deletingId === n.id ? 'Removing…' : 'Remove' }}
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>

    <!-- Create / edit dialog -->
    <Dialog v-model:open="dialogOpen">
      <DialogHeader>
        <DialogTitle>{{ isEdit ? 'Edit Node' : 'Register Node' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="node-name">Name</Label>
          <Input id="node-name" v-model="form.name" placeholder="mta-eu-2" class="font-mono" />
          <p class="text-caption text-medium-emphasis">
            DNS-safe label; it becomes the node identity in logs and metrics.
          </p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="node-agent">Agent URL</Label>
          <Input id="node-agent" v-model="form.agentUrl" placeholder="https://10.20.0.12:8447" class="font-mono" />
          <p v-if="agentUrlError" class="text-caption text-error">{{ agentUrlError }}</p>
          <p v-else class="text-caption text-medium-emphasis">
            Leave empty for the co-located node managed through the local filesystem.
          </p>
        </div>
        <div class="d-flex ga-3">
          <div class="d-flex flex-column ga-1 flex-grow-1">
            <Label for="node-proxy-host">kumo-proxy host</Label>
            <Input id="node-proxy-host" v-model="form.proxyHost" placeholder="10.20.0.12" class="font-mono" />
          </div>
          <div class="d-flex flex-column ga-1" style="width: 120px">
            <Label for="node-proxy-port">Port</Label>
            <Input id="node-proxy-port" v-model="form.proxyPort" placeholder="1080" class="font-mono" />
          </div>
        </div>
        <p v-if="proxyError" class="text-caption text-error">{{ proxyError }}</p>
        <p v-else class="text-caption text-medium-emphasis">
          Private-network IP only — kumo-proxy is unauthenticated and must never be publicly reachable.
        </p>
        <div class="d-flex flex-column ga-1">
          <Label>Status</Label>
          <v-select
            v-model="form.status"
            :items="statusOptions"
            variant="outlined"
            density="compact"
            hide-details
            data-testid="node-status"
          />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="node-notes">Notes</Label>
          <Input id="node-notes" v-model="form.notes" placeholder="Optional operator notes" />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !canSubmit">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Register' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
