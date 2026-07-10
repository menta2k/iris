<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import StatTile from '@/components/dashboard/StatTile.vue'
import { Card, CardContent } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { monitoringService } from '@/services'
import { ApiError } from '@/services/http'
import type { MonitoringAccount, MonitoringProtocol, MonitoringProvider } from '@/types'

const router = useRouter()
const { items, loading, error, notImplemented, load } = useAsyncList<MonitoringAccount>({
  loader: () => monitoringService.listAccounts(),
})
const { toast } = useToast()

// Provider presets: choosing a provider prefills host/port/protocol so operators
// rarely touch the connection details. "custom" leaves them editable.
interface Preset {
  host: string
  port: number
  protocol: MonitoringProtocol
  tls: boolean
}
const PRESETS: Record<Exclude<MonitoringProvider, 'custom'>, Preset> = {
  gmail: { host: 'imap.gmail.com', port: 993, protocol: 'imap', tls: true },
  outlook: { host: 'outlook.office365.com', port: 993, protocol: 'imap', tls: true },
  yahoo: { host: 'imap.mail.yahoo.com', port: 993, protocol: 'imap', tls: true },
}
const providerOptions = [
  { title: 'Gmail', value: 'gmail' },
  { title: 'Outlook / Microsoft 365', value: 'outlook' },
  { title: 'Yahoo', value: 'yahoo' },
  { title: 'Custom', value: 'custom' },
]
const protocolOptions = [
  { title: 'IMAP', value: 'imap' },
  { title: 'POP3', value: 'pop3' },
]

const stats = computed(() => {
  const list = items.value
  return {
    total: list.length,
    enabled: list.filter((a) => a.enabled).length,
    scheduled: list.filter((a) => a.scheduleEnabled).length,
    ready: list.filter((a) => a.hasPassword).length,
  }
})

type BadgeVariant = 'default' | 'secondary' | 'destructive' | 'success' | 'warning'
const SEND_VARIANT: Record<string, BadgeVariant> = {
  queued: 'secondary',
  sent: 'success',
  deferred: 'warning',
  bounced: 'destructive',
  error: 'destructive',
}
const PLACEMENT_VARIANT: Record<string, BadgeVariant> = {
  inbox: 'success',
  spam: 'warning',
  missing: 'destructive',
  unknown: 'secondary',
}

function formatDate(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(undefined, { hour12: false })
}

// ---- Create / edit dialog ----
const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)

interface FormState {
  label: string
  provider: MonitoringProvider
  email: string
  protocol: MonitoringProtocol
  host: string
  port: number
  tls: boolean
  username: string
  password: string
  checkFolders: string[]
  fromAddress: string
  scheduleEnabled: boolean
  scheduleInterval: string
  fetchDelay: string
  enabled: boolean
}
function emptyForm(): FormState {
  return {
    label: '',
    provider: 'gmail',
    email: '',
    protocol: 'imap',
    host: PRESETS.gmail.host,
    port: 993,
    tls: true,
    username: '',
    password: '',
    checkFolders: ['INBOX'],
    fromAddress: '',
    scheduleEnabled: false,
    scheduleInterval: '6h',
    fetchDelay: '10m',
    enabled: true,
  }
}
const form = ref<FormState>(emptyForm())
const isEdit = computed(() => mode.value === 'edit')

function applyPreset(provider: MonitoringProvider) {
  if (provider === 'custom') return
  const p = PRESETS[provider]
  form.value.host = p.host
  form.value.port = p.port
  form.value.protocol = p.protocol
  form.value.tls = p.tls
}

const canSubmit = computed(
  () =>
    !!form.value.label.trim() &&
    !!form.value.email.trim() &&
    !!form.value.host.trim() &&
    form.value.port > 0 &&
    (isEdit.value || !!form.value.password),
)

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = emptyForm()
  testResult.value = null
  dialogOpen.value = true
}
function openEdit(a: MonitoringAccount) {
  mode.value = 'edit'
  editId.value = a.id
  form.value = {
    label: a.label,
    provider: a.provider,
    email: a.email,
    protocol: a.protocol,
    host: a.host,
    port: a.port,
    tls: a.tls,
    username: a.username,
    password: '',
    checkFolders: [...a.checkFolders],
    fromAddress: a.fromAddress,
    scheduleEnabled: a.scheduleEnabled,
    scheduleInterval: a.scheduleInterval || '6h',
    fetchDelay: a.fetchDelay || '10m',
    enabled: a.enabled,
  }
  testResult.value = null
  dialogOpen.value = true
}

// ---- Test connection (on-demand; never blocks saving) ----
const testing = ref(false)
const testResult = ref<{ ok: boolean; error?: string } | null>(null)
async function testConnection() {
  testResult.value = null
  testing.value = true
  try {
    const res = await monitoringService.verify({
      id: editId.value ?? undefined,
      protocol: form.value.protocol,
      host: form.value.host.trim(),
      port: Number(form.value.port),
      tls: form.value.tls,
      username: form.value.username.trim(),
      email: form.value.email.trim(),
      password: form.value.password,
    })
    testResult.value = { ok: res.ok, error: res.error }
  } catch (err) {
    testResult.value = { ok: false, error: err instanceof ApiError ? err.message : 'Verification failed.' }
  } finally {
    testing.value = false
  }
}

async function submit() {
  if (!canSubmit.value) return
  saving.value = true
  try {
    const common = {
      label: form.value.label.trim(),
      provider: form.value.provider,
      email: form.value.email.trim(),
      protocol: form.value.protocol,
      host: form.value.host.trim(),
      port: Number(form.value.port),
      tls: form.value.tls,
      username: form.value.username.trim(),
      checkFolders: form.value.checkFolders,
      fromAddress: form.value.fromAddress.trim(),
      scheduleEnabled: form.value.scheduleEnabled,
      scheduleInterval: form.value.scheduleInterval.trim(),
      fetchDelay: form.value.fetchDelay.trim(),
      enabled: form.value.enabled,
    }
    if (isEdit.value && editId.value) {
      await monitoringService.updateAccount(editId.value, { id: editId.value, ...common })
      toast({ title: 'Account updated', description: common.email, variant: 'success' })
    } else {
      await monitoringService.createAccount({ ...common, password: form.value.password })
      toast({ title: 'Account added', description: common.email, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save account.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

// ---- Reset password dialog ----
const pwDialogOpen = ref(false)
const pwSaving = ref(false)
const pwTarget = ref<MonitoringAccount | null>(null)
const newPassword = ref('')
function openResetPassword(a: MonitoringAccount) {
  pwTarget.value = a
  newPassword.value = ''
  pwDialogOpen.value = true
}
async function submitPassword() {
  if (!pwTarget.value || !newPassword.value) return
  pwSaving.value = true
  try {
    await monitoringService.setPassword(pwTarget.value.id, newPassword.value)
    toast({ title: 'Password saved', description: pwTarget.value.email, variant: 'success' })
    pwDialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save password.'
    toast({ title: 'Save failed', description: msg, variant: 'destructive' })
  } finally {
    pwSaving.value = false
  }
}

// ---- Send test probe ----
const sendingId = ref<string | null>(null)
async function sendProbe(a: MonitoringAccount) {
  sendingId.value = a.id
  try {
    await monitoringService.sendProbe(a.id)
    toast({ title: 'Probe sent', description: `Queued a probe to ${a.email}`, variant: 'success' })
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to send probe.'
    toast({ title: 'Send failed', description: msg, variant: 'destructive' })
  } finally {
    sendingId.value = null
  }
}

// ---- Delete ----
const deletingId = ref<string | null>(null)
async function remove(a: MonitoringAccount) {
  deletingId.value = a.id
  try {
    await monitoringService.removeAccount(a.id)
    toast({ title: 'Account removed', description: a.email, variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to delete account.'
    toast({ title: 'Delete failed', description: msg, variant: 'destructive' })
  } finally {
    deletingId.value = null
  }
}

function viewProbes(a: MonitoringAccount) {
  router.push({ name: 'inbox-probes', params: { id: a.id } })
}
</script>

<template>
  <div>
    <PageHeader
      title="ESP Monitoring"
      description="Mailbox accounts iris sends seed (probe) mail to and later inspects for inbox placement. Add a mailbox with IMAP/POP3 credentials, then send a probe manually or on a recurring schedule. Passwords are stored encrypted and never shown again."
    >
      <template #actions>
        <Button data-testid="create-account" @click="openCreate">Add Mailbox</Button>
      </template>
    </PageHeader>

    <v-row dense class="mb-2">
      <v-col cols="12" sm="6" lg="3">
        <StatTile label="Mailboxes" :value="stats.total.toLocaleString()" caption="Monitored accounts" icon="mdi-email-search-outline" color="primary" />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile label="Enabled" :value="stats.enabled.toLocaleString()" caption="Accepting probes" icon="mdi-check-circle-outline" color="success" />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile label="Scheduled" :value="stats.scheduled.toLocaleString()" caption="Recurring probes" icon="mdi-timer-outline" color="info" />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile label="Credentialed" :value="stats.ready.toLocaleString()" caption="Password stored" icon="mdi-key-outline" :color="stats.ready < stats.total ? 'warning' : 'secondary'" />
      </v-col>
    </v-row>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No mailboxes yet. Add one to start monitoring inbox placement."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Label</TableHead>
                <TableHead>Mailbox</TableHead>
                <TableHead>Connection</TableHead>
                <TableHead>Schedule</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Last probe</TableHead>
                <TableHead>Last result</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="a in items" :key="a.id">
                <TableCell class="font-weight-medium">{{ a.label }}</TableCell>
                <TableCell class="font-mono text-caption">{{ a.email }}</TableCell>
                <TableCell class="text-caption text-medium-emphasis">
                  {{ a.protocol.toUpperCase() }} · {{ a.host }}:{{ a.port }}
                  <Badge v-if="!a.tls" variant="secondary" class="ml-1">no TLS</Badge>
                </TableCell>
                <TableCell>
                  <Badge v-if="a.scheduleEnabled" variant="default">every {{ a.scheduleInterval }}</Badge>
                  <span v-else class="text-caption text-medium-emphasis">Manual</span>
                </TableCell>
                <TableCell>
                  <div class="d-flex ga-1">
                    <Badge :variant="a.enabled ? 'success' : 'secondary'">{{ a.enabled ? 'Enabled' : 'Disabled' }}</Badge>
                    <Badge v-if="!a.hasPassword" variant="destructive">No password</Badge>
                  </div>
                </TableCell>
                <TableCell class="text-caption text-no-wrap">{{ formatDate(a.lastProbeAt) }}</TableCell>
                <TableCell>
                  <div v-if="a.lastProbeSendStatus" class="d-flex ga-1 align-center">
                    <Badge :variant="SEND_VARIANT[a.lastProbeSendStatus] ?? 'secondary'">{{ a.lastProbeSendStatus }}</Badge>
                    <Badge v-if="a.lastProbePlacement" :variant="PLACEMENT_VARIANT[a.lastProbePlacement] ?? 'secondary'">{{ a.lastProbePlacement }}</Badge>
                    <span v-else-if="a.lastProbeMailboxStatus && a.lastProbeMailboxStatus !== 'found'" class="text-caption text-medium-emphasis">{{ a.lastProbeMailboxStatus }}</span>
                  </div>
                  <span v-else class="text-caption text-medium-emphasis">—</span>
                </TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-2">
                    <Button
                      size="sm"
                      :disabled="sendingId === a.id || !a.enabled"
                      :data-testid="`send-${a.id}`"
                      @click="sendProbe(a)"
                    >
                      {{ sendingId === a.id ? 'Sending…' : 'Send test' }}
                    </Button>
                    <Button variant="outline" size="sm" :data-testid="`probes-${a.id}`" @click="viewProbes(a)">Probes</Button>
                    <Button variant="outline" size="sm" :data-testid="`password-${a.id}`" @click="openResetPassword(a)">Password</Button>
                    <Button variant="outline" size="sm" :data-testid="`edit-${a.id}`" @click="openEdit(a)">Edit</Button>
                    <Button
                      variant="outline"
                      size="sm"
                      :disabled="deletingId === a.id"
                      :data-testid="`delete-${a.id}`"
                      @click="remove(a)"
                    >
                      {{ deletingId === a.id ? 'Removing…' : 'Remove' }}
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
        <DialogTitle>{{ isEdit ? 'Edit Mailbox' : 'Add Mailbox' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="mon-label">Label</Label>
          <Input id="mon-label" v-model="form.label" placeholder="Gmail seed inbox" />
        </div>
        <v-row dense>
          <v-col cols="12" sm="6">
            <Label>Provider</Label>
            <v-select
              v-model="form.provider"
              :items="providerOptions"
              variant="outlined"
              density="compact"
              hide-details
              data-testid="mon-provider"
              @update:model-value="applyPreset"
            />
          </v-col>
          <v-col cols="12" sm="6">
            <Label>Protocol</Label>
            <v-select
              v-model="form.protocol"
              :items="protocolOptions"
              :disabled="form.provider !== 'custom'"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
        </v-row>
        <div class="d-flex flex-column ga-1">
          <Label for="mon-email">Mailbox address</Label>
          <Input id="mon-email" v-model="form.email" placeholder="seed@gmail.com" class="font-mono" />
        </div>
        <v-row dense>
          <v-col cols="8">
            <Label for="mon-host">Host</Label>
            <Input id="mon-host" v-model="form.host" :disabled="form.provider !== 'custom'" class="font-mono" />
          </v-col>
          <v-col cols="4">
            <Label for="mon-port">Port</Label>
            <Input id="mon-port" v-model.number="form.port" type="number" :disabled="form.provider !== 'custom'" />
          </v-col>
        </v-row>
        <div class="d-flex align-center ga-2">
          <v-switch v-model="form.tls" color="primary" density="compact" hide-details inset :disabled="form.provider !== 'custom'" />
          <span class="text-body-2">Use TLS</span>
        </div>
        <v-row dense>
          <v-col cols="12" sm="6">
            <Label for="mon-username">Username</Label>
            <Input id="mon-username" v-model="form.username" placeholder="defaults to mailbox address" class="font-mono" />
          </v-col>
          <v-col v-if="!isEdit" cols="12" sm="6">
            <Label for="mon-password">Password</Label>
            <Input id="mon-password" v-model="form.password" type="password" placeholder="app password" />
          </v-col>
        </v-row>
        <div class="d-flex flex-column ga-1">
          <Label>Folders to search (IMAP)</Label>
          <v-combobox
            v-model="form.checkFolders"
            multiple
            chips
            closable-chips
            variant="outlined"
            density="compact"
            hide-details
            placeholder="INBOX, Spam…"
            data-testid="mon-folders"
          />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="mon-from">From address (probe sender)</Label>
          <Input id="mon-from" v-model="form.fromAddress" placeholder="leave blank to use IRIS_MONITORING_FROM" class="font-mono" />
        </div>
        <div class="d-flex align-center ga-2">
          <v-switch v-model="form.scheduleEnabled" color="primary" density="compact" hide-details inset data-testid="mon-schedule" />
          <span class="text-body-2">Send probes on a recurring schedule</span>
        </div>
        <v-row v-if="form.scheduleEnabled" dense>
          <v-col cols="12" sm="6">
            <Label for="mon-interval">Interval</Label>
            <Input id="mon-interval" v-model="form.scheduleInterval" placeholder="6h" class="font-mono" />
          </v-col>
          <v-col cols="12" sm="6">
            <Label for="mon-delay">Fetch delay</Label>
            <Input id="mon-delay" v-model="form.fetchDelay" placeholder="10m" class="font-mono" />
          </v-col>
        </v-row>
        <div class="d-flex align-center ga-2">
          <v-switch v-model="form.enabled" color="primary" density="compact" hide-details inset />
          <span class="text-body-2">{{ form.enabled ? 'Enabled' : 'Disabled' }}</span>
        </div>
        <div class="d-flex align-center ga-3">
          <Button
            type="button"
            variant="outline"
            size="sm"
            :disabled="testing || !form.host.trim() || form.port <= 0"
            data-testid="test-connection"
            @click="testConnection"
          >
            {{ testing ? 'Testing…' : 'Test connection' }}
          </Button>
          <span v-if="testResult?.ok" class="text-caption text-success d-flex align-center ga-1">
            <v-icon size="small" icon="mdi-check-circle-outline" /> Connection OK
          </span>
          <span v-else-if="testResult" class="text-caption text-error d-flex align-center ga-1">
            <v-icon size="small" icon="mdi-alert-circle-outline" /> {{ testResult.error || 'Connection failed' }}
          </span>
          <span v-else class="text-caption text-medium-emphasis">
            {{ isEdit ? 'Tests the stored password unless you enter a new one.' : 'Verifies login before saving.' }}
          </span>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !canSubmit">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Add' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>

    <!-- Password dialog -->
    <Dialog v-model:open="pwDialogOpen">
      <DialogHeader>
        <DialogTitle>Set mailbox password</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submitPassword">
        <p class="text-body-2 text-medium-emphasis">
          Store an encrypted password for <code class="font-mono">{{ pwTarget?.email }}</code>. For Gmail/Yahoo use
          an app password, not the account password.
        </p>
        <div class="d-flex flex-column ga-1">
          <Label for="mon-newpw">Password</Label>
          <Input id="mon-newpw" v-model="newPassword" type="password" placeholder="app password" />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="pwDialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="pwSaving || !newPassword">{{ pwSaving ? 'Saving…' : 'Save' }}</Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
