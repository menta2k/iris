<script setup lang="ts">
import { computed, ref, watch } from 'vue'
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
import { StatusBadge, Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { acmeService, clusterService, outboundConfigService } from '@/services'
import { ApiError } from '@/services/http'
import type { AcmeCertificate, Listener, ListenerRole, MTANode } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<Listener>({
  loader: () => outboundConfigService.listListeners(),
})

// Cluster nodes for the optional bind-node selector ('' = every node).
const availableNodes = ref<MTANode[]>([])
const nodeItems = computed(() => [
  { title: '\u2014 Every node \u2014', value: '' },
  ...availableNodes.value.map((n) => ({ title: n.name, value: n.id })),
])
async function loadNodes() {
  try {
    const res = await clusterService.listNodes()
    availableNodes.value = (res.items ?? []).filter((n) => n.status !== 'disabled')
  } catch {
    availableNodes.value = []
  }
}

// Assignable IPs of the selected node (local host for "every node"), shown as a
// dropdown for the IP field. Failure (agent down) leaves it empty — the
// combobox still accepts a typed address.
const nodeIPs = ref<string[]>([])
const ipsLoading = ref(false)
async function loadNodeIPs(nodeId: string) {
  ipsLoading.value = true
  try {
    const res = await clusterService.nodeIPs(nodeId)
    nodeIPs.value = res.ips ?? []
  } catch {
    nodeIPs.value = []
  } finally {
    ipsLoading.value = false
  }
}
// ACME-managed certificates, offered as dropdown options for the TLS cert/key
// paths (like the node IP picker). The comboboxes stay editable, so a
// manually-managed path can still be typed. Failure leaves the lists empty.
const acmeCerts = ref<AcmeCertificate[]>([])
const certPathItems = computed(() =>
  acmeCerts.value.map((c) => c.certPath).filter(Boolean),
)
const keyPathItems = computed(() =>
  acmeCerts.value.map((c) => c.keyPath).filter(Boolean),
)
async function loadAcmeCerts() {
  try {
    const res = await acmeService.listCertificates()
    acmeCerts.value = res.items ?? []
  } catch {
    acmeCerts.value = []
  }
}
// Selecting a known cert path auto-fills its paired key path — an ACME cert
// ships fullchain + privkey together, so choosing one implies the other.
function onCertPathChange(path: string) {
  const match = acmeCerts.value.find((c) => c.certPath === path)
  if (match?.keyPath) form.value.tls_key_path = match.keyPath
}

const { toast } = useToast()

const LISTENER_STATUSES = ['active', 'disabled']

// v-select item lists ({ title, value }).
const listenerStatusItems = LISTENER_STATUSES.map((s) => ({ title: s, value: s }))
const listenerRoleItems: Array<{ title: string; value: ListenerRole }> = [
  { title: 'Inbound (MX — receives mail, no relay)', value: 'inbound' },
  { title: 'Submission (authorized senders relay outbound)', value: 'submission' },
]

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)

// relay_hosts is edited as free text (comma/newline-separated) and split on submit.
function emptyForm() {
  return {
    name: '',
    ip_address: '',
    port: 25,
    hostname: '',
    tls_enabled: false,
    tls_cert_path: '',
    tls_key_path: '',
    require_auth: false,
    max_message_size: '0',
    relay_hosts_text: '',
    status: 'active',
    role: 'inbound' as ListenerRole,
    node_id: '',
  }
}
const form = ref(emptyForm())
// Refresh the IP dropdown whenever the bind-node changes.
watch(() => form.value.node_id, (id) => loadNodeIPs(id))

const isEdit = computed(() => mode.value === 'edit')

// Split the free-text relay hosts on commas/newlines; blank yields an empty
// array (loopback-only, i.e. an inbound/MX listener).
function parseRelayHosts(text: string): string[] {
  return text
    .split(/[\n,]+/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
}

// Picking a role suggests its conventional port, but only when the current port
// is still the other role's default (never clobber an explicit port).
function onRoleChange() {
  if (form.value.role === 'submission' && Number(form.value.port) === 25) form.value.port = 587
  if (form.value.role === 'inbound' && Number(form.value.port) === 587) form.value.port = 25
}

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = emptyForm()
  dialogOpen.value = true
  void loadNodes()
  void loadNodeIPs(form.value.node_id)
  void loadAcmeCerts()
}

function openEdit(l: Listener) {
  mode.value = 'edit'
  editId.value = l.id
  form.value = {
    name: l.name,
    ip_address: l.ipAddress,
    port: l.port,
    hostname: l.hostname,
    tls_enabled: l.tlsEnabled,
    tls_cert_path: l.tlsCertPath ?? '',
    tls_key_path: l.tlsKeyPath ?? '',
    require_auth: l.requireAuth,
    max_message_size: l.maxMessageSize ?? '0',
    relay_hosts_text: (l.relayHosts ?? []).join('\n'),
    status: (l.status || 'active').toLowerCase(),
    role: l.role ?? 'inbound',
    node_id: l.nodeId ?? '',
  }
  dialogOpen.value = true
  void loadNodes()
  void loadNodeIPs(form.value.node_id)
  void loadAcmeCerts()
}

async function submit() {
  if (!form.value.name || !form.value.ip_address || !form.value.hostname) return
  saving.value = true
  const body = {
    name: form.value.name,
    ip_address: form.value.ip_address,
    port: Number(form.value.port),
    hostname: form.value.hostname,
    tls_enabled: form.value.tls_enabled,
    tls_cert_path: form.value.tls_cert_path,
    tls_key_path: form.value.tls_key_path,
    require_auth: form.value.require_auth,
    max_message_size: String(form.value.max_message_size || '0'),
    relay_hosts: parseRelayHosts(form.value.relay_hosts_text),
    role: form.value.role,
    node_id: form.value.node_id,
  }
  try {
    if (isEdit.value && editId.value) {
      await outboundConfigService.updateListener(editId.value, {
        ...body,
        status: form.value.status,
      })
      toast({ title: 'Listener updated', description: form.value.name, variant: 'success' })
    } else {
      await outboundConfigService.createListener(body)
      toast({ title: 'Listener created', description: form.value.name, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save listener.'
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
    <PageHeader
      title="Listeners"
      description="ESMTP listeners: IP, port, EHLO hostname and TLS/relay config."
    >
      <template #actions>
        <Button data-testid="create-listener" @click="openCreate">New Listener</Button>
      </template>
    </PageHeader>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No listeners configured yet."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>IP:Port</TableHead>
                <TableHead>Hostname</TableHead>
                <TableHead>Node</TableHead>
                <TableHead>TLS</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="l in items" :key="l.id">
                <TableCell class="font-medium">{{ l.name }}</TableCell>
                <TableCell class="font-mono text-xs">{{ l.ipAddress }}:{{ l.port }}</TableCell>
                <TableCell>{{ l.hostname }}</TableCell>
                <TableCell>{{ l.nodeName || 'every node' }}</TableCell>
                <TableCell>
                  <Badge :variant="l.tlsEnabled ? 'secondary' : 'outline'">
                    {{ l.tlsEnabled ? 'on' : 'off' }}
                  </Badge>
                </TableCell>
                <TableCell><StatusBadge :status="l.status" /></TableCell>
                <TableCell class="text-right">
                  <Button
                    variant="outline"
                    size="sm"
                    :data-testid="`edit-listener-${l.id}`"
                    @click="openEdit(l)"
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
        <DialogTitle>{{ isEdit ? 'Edit Listener' : 'Create Listener' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="listener-name">Name</Label>
          <Input id="listener-name" v-model="form.name" placeholder="esmtp-east-1" />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="listener-role">Role</Label>
          <v-select
            id="listener-role"
            data-testid="listener-role"
            :model-value="form.role"
            :items="listenerRoleItems"
            variant="outlined"
            density="compact"
            hide-details
            @update:model-value="
              (v: ListenerRole) => {
                form.role = v
                onRoleChange()
              }
            "
          />
          <p class="text-caption text-medium-emphasis">
            <strong>Inbound</strong> accepts mail for local domains and must leave the relay
            allowlist empty. <strong>Submission</strong> requires at least one relay host. Loopback
            always relays. (Picking a role suggests its conventional port.)
          </p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="listener-node">Bind on node</Label>
          <v-select
            id="listener-node"
            v-model="form.node_id"
            data-testid="listener-node"
            :items="nodeItems"
            variant="outlined"
            density="compact"
            hide-details
          />
          <p class="text-caption text-medium-emphasis">
            Pin this listener to one cluster node so it binds only there (its IP must be that
            node's address). <strong>Every node</strong> binds it on all nodes from the one shared
            policy — use a floating/shared IP for that.
          </p>
        </div>
        <v-row dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="listener-ip">IP Address</Label>
            <v-combobox
              id="listener-ip"
              v-model="form.ip_address"
              data-testid="listener-ip"
              :items="nodeIPs"
              :loading="ipsLoading"
              variant="outlined"
              density="compact"
              hide-details
              placeholder="203.0.113.10"
            />
            <p class="text-caption text-medium-emphasis">
              Pick an address detected on
              {{ form.node_id ? "the selected node" : "this host" }}, or type one. A concrete IP — not 0.0.0.0.
            </p>
          </v-col>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="listener-port">Port</Label>
            <Input id="listener-port" v-model.number="form.port" type="number" placeholder="25" />
          </v-col>
        </v-row>
        <div class="d-flex flex-column ga-1">
          <Label for="listener-hostname">Hostname (EHLO)</Label>
          <Input id="listener-hostname" v-model="form.hostname" placeholder="mail.example.com" />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label class="flex items-center gap-2">
            <input
              id="listener-tls"
              v-model="form.tls_enabled"
              type="checkbox"
              data-testid="listener-tls"
            />
            Enable TLS
          </Label>
        </div>
        <template v-if="form.tls_enabled">
          <div class="d-flex flex-column ga-1">
            <Label for="listener-cert">TLS Certificate Path</Label>
            <v-combobox
              id="listener-cert"
              v-model="form.tls_cert_path"
              data-testid="listener-cert"
              :items="certPathItems"
              variant="outlined"
              density="compact"
              hide-details
              placeholder="/opt/kumomta/etc/tls/example.com/fullchain.pem"
              @update:model-value="onCertPathChange"
            />
            <p class="text-caption text-medium-emphasis">
              Pick an ACME-managed certificate, or type a path. Selecting one fills the key below.
            </p>
          </div>
          <div class="d-flex flex-column ga-1">
            <Label for="listener-key">TLS Key Path</Label>
            <v-combobox
              id="listener-key"
              v-model="form.tls_key_path"
              data-testid="listener-key"
              :items="keyPathItems"
              variant="outlined"
              density="compact"
              hide-details
              placeholder="/opt/kumomta/etc/tls/example.com/privkey.pem"
            />
          </div>
        </template>
        <v-row dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="listener-max-size">Max Message Size (bytes)</Label>
            <Input
              id="listener-max-size"
              v-model="form.max_message_size"
              type="number"
              placeholder="0"
            />
            <p class="text-caption text-medium-emphasis">0 = unlimited.</p>
          </v-col>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label class="flex items-center gap-2 pt-7">
              <input id="listener-auth" v-model="form.require_auth" type="checkbox" />
              Require Auth
            </Label>
          </v-col>
        </v-row>
        <div class="d-flex flex-column ga-1">
          <Label for="listener-relay">Relay allowlist (IPs / CIDRs)</Label>
          <textarea
            id="listener-relay"
            v-model="form.relay_hosts_text"
            rows="3"
            class="d-flex w-100 rounded border border-input bg-background px-3 py-1 text-body-2 shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            placeholder="10.1.111.0/24, 192.168.1.5"
          ></textarea>
          <p class="text-caption text-medium-emphasis">
            Hosts allowed to relay (submit outbound) through this listener — comma/newline-separated.
            Loopback (127.0.0.1) is always allowed. <strong>Leave blank for loopback-only</strong>
            (inbound-only / MX listener that otherwise accepts mail only for local domains). Add CIDRs
            on a submission listener (e.g. :587) to authorize other senders.
          </p>
        </div>
        <div v-if="isEdit" class="d-flex flex-column ga-1">
          <Label for="listener-status">Status</Label>
          <v-select
            id="listener-status"
            v-model="form.status"
            :items="listenerStatusItems"
            variant="outlined"
            density="compact"
            hide-details
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button
            type="submit"
            :disabled="saving || !form.name || !form.ip_address || !form.hostname"
          >
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Create' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
