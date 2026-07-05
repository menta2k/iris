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
import { StatusBadge, Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { inboundAutomationService } from '@/services'
import { ApiError } from '@/services/http'
import type {
  ForwardTLS,
  InboundRoute,
  InboundRouteAction,
  InboundRouteMatchType,
  InboundRouteRequest,
  SpamScanMode,
} from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<InboundRoute>({
  loader: () => inboundAutomationService.listInboundRoutes(),
})
const { toast } = useToast()

const ACTIONS: InboundRouteAction[] = ['maildir', 'forward', 'webhook']
const STATUSES = ['active', 'disabled']
const FORWARD_TLS: ForwardTLS[] = ['opportunistic', 'required', 'none']
const SPAM_SCANS: SpamScanMode[] = ['default', 'off', 'tag', 'enforce']
const MATCH_TYPES: InboundRouteMatchType[] = ['recipient_email', 'recipient_domain']

// v-select item lists ({ title, value }) derived from the enums above.
const actionItems = ACTIONS.map((a) => ({ title: a, value: a }))
const statusItems = STATUSES.map((s) => ({ title: s, value: s }))
const forwardTlsItems = FORWARD_TLS.map((t) => ({ title: t, value: t }))
const spamScanItems = SPAM_SCANS.map((m) => ({ title: m, value: m }))
const matchTypeItems = MATCH_TYPES.map((m) => ({ title: m, value: m }))

interface RouteForm {
  name: string
  match_type: InboundRouteMatchType
  match_value: string
  action: InboundRouteAction
  priority: number
  status: string
  spam_scan: SpamScanMode
  forward_host: string
  forward_port: number
  forward_tls: ForwardTLS
  maildir_path: string
  destination_url: string
  timeout_seconds: number
  secret_ref: string
}

function emptyForm(): RouteForm {
  return {
    name: '',
    match_type: 'recipient_domain',
    match_value: '',
    action: 'maildir',
    priority: 0,
    status: 'active',
    spam_scan: 'default',
    forward_host: '',
    forward_port: 25,
    forward_tls: 'opportunistic',
    maildir_path: '',
    destination_url: '',
    timeout_seconds: 10,
    secret_ref: '',
  }
}

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref<RouteForm>(emptyForm())

const isEdit = computed(() => mode.value === 'edit')

// Colour-code the Action column so the delivery type is scannable at a glance:
// maildir (local storage) → green, forward (relay) → indigo, webhook (external
// HTTP) → amber. Unknown actions fall back to neutral.
const ACTION_VARIANT: Record<InboundRouteAction, 'success' | 'default' | 'warning'> = {
  maildir: 'success',
  forward: 'default',
  webhook: 'warning',
}
function actionVariant(action: string) {
  return ACTION_VARIANT[action as InboundRouteAction] ?? 'secondary'
}

// Colour-code the Scan column by rspamd mode: enforce (rejects spam) → green,
// tag (adds headers) → amber, off (no scanning) → red, default (inherits the
// global mode) → neutral.
const SCAN_VARIANT: Record<SpamScanMode, 'success' | 'warning' | 'destructive' | 'secondary'> = {
  enforce: 'success',
  tag: 'warning',
  off: 'destructive',
  default: 'secondary',
}
function scanVariant(scan: string) {
  return SCAN_VARIANT[scan as SpamScanMode] ?? 'secondary'
}

function summarizeTarget(r: InboundRoute): string {
  switch (r.action) {
    case 'forward':
      return `${r.forwardHost}:${r.forwardPort} (${r.forwardTls})`
    case 'maildir':
      return r.maildirPath || '(default base)'
    case 'webhook':
      return r.destinationUrl
    default:
      return ''
  }
}

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = emptyForm()
  dialogOpen.value = true
}

function openEdit(r: InboundRoute) {
  mode.value = 'edit'
  editId.value = r.id
  form.value = {
    name: r.name,
    match_type: (r.matchType as InboundRouteMatchType) || 'recipient_domain',
    match_value: r.matchValue,
    action: (r.action as InboundRouteAction) || 'maildir',
    priority: r.priority,
    status: (r.status || 'active').toLowerCase(),
    spam_scan: (r.spamScan as SpamScanMode) || 'default',
    forward_host: r.forwardHost,
    forward_port: r.forwardPort || 25,
    forward_tls: (r.forwardTls as ForwardTLS) || 'opportunistic',
    maildir_path: r.maildirPath,
    destination_url: r.destinationUrl,
    timeout_seconds: r.timeoutSeconds || 10,
    // Never display the existing secret; blank preserves it.
    secret_ref: '',
  }
  dialogOpen.value = true
}

const canSubmit = computed(() => {
  if (!form.value.name || !form.value.match_value) return false
  if (form.value.action === 'forward') return !!form.value.forward_host
  if (form.value.action === 'webhook') return !!form.value.destination_url
  return true
})

function payload(): InboundRouteRequest {
  return {
    name: form.value.name,
    match_type: form.value.match_type,
    match_value: form.value.match_value,
    action: form.value.action,
    priority: Number(form.value.priority) || 0,
    status: form.value.status,
    spam_scan: form.value.spam_scan,
    forward_host: form.value.forward_host,
    forward_port: Number(form.value.forward_port) || 25,
    forward_tls: form.value.forward_tls,
    maildir_path: form.value.maildir_path,
    destination_url: form.value.destination_url,
    timeout_seconds: Number(form.value.timeout_seconds) || 10,
    secret_ref: form.value.secret_ref,
  }
}

async function submit() {
  if (!canSubmit.value) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await inboundAutomationService.updateInboundRoute(editId.value, payload())
      toast({ title: 'Inbound route updated', description: form.value.name, variant: 'success' })
    } else {
      await inboundAutomationService.createInboundRoute(payload())
      toast({ title: 'Inbound route created', description: form.value.name, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save inbound route.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

async function remove(r: InboundRoute) {
  if (!window.confirm(`Delete inbound route "${r.name}"?`)) return
  try {
    await inboundAutomationService.deleteInboundRoute(r.id)
    toast({ title: 'Inbound route deleted', description: r.name, variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to delete inbound route.'
    toast({ title: 'Delete failed', description: msg, variant: 'destructive' })
  }
}
</script>

<template>
  <div>
    <PageHeader
      title="Inbound Routes"
      description="Route inbound mail for domains you host to a maildir, a forwarding smarthost, or a webhook."
    >
      <template #actions>
        <Button data-testid="create-inbound-route" @click="openCreate">New Route</Button>
      </template>
    </PageHeader>
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No inbound routes configured."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Match</TableHead>
                <TableHead>Target</TableHead>
                <TableHead>Scan</TableHead>
                <TableHead>Priority</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="r in items" :key="r.id">
                <TableCell class="font-medium">{{ r.name }}</TableCell>
                <TableCell><Badge :variant="actionVariant(r.action)">{{ r.action }}</Badge></TableCell>
                <TableCell class="font-mono text-caption">{{ r.matchType }}: {{ r.matchValue }}</TableCell>
                <TableCell class="font-mono text-caption">{{ summarizeTarget(r) }}</TableCell>
                <TableCell><Badge :variant="scanVariant(r.spamScan || 'default')">{{ r.spamScan || 'default' }}</Badge></TableCell>
                <TableCell class="tabular-nums">{{ r.priority }}</TableCell>
                <TableCell><StatusBadge :status="r.status" /></TableCell>
                <TableCell class="space-x-2 text-right">
                  <Button
                    variant="outline"
                    size="sm"
                    :data-testid="`edit-inbound-route-${r.id}`"
                    @click="openEdit(r)"
                  >
                    Edit
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    :data-testid="`delete-inbound-route-${r.id}`"
                    @click="remove(r)"
                  >
                    Delete
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
        <DialogTitle>{{ isEdit ? 'Edit Inbound Route' : 'Create Inbound Route' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="ir-name">Name</Label>
          <Input id="ir-name" v-model="form.name" placeholder="archive-inbound" />
        </div>
        <v-row dense>
          <v-col cols="6">
            <div class="d-flex flex-column ga-1">
              <Label for="ir-match-type">Match Type</Label>
              <v-select
                id="ir-match-type"
                v-model="form.match_type"
                :items="matchTypeItems"
                variant="outlined"
                density="compact"
                hide-details
              />
            </div>
          </v-col>
          <v-col cols="6">
            <div class="d-flex flex-column ga-1">
              <Label for="ir-match-value">Match Value</Label>
              <Input id="ir-match-value" v-model="form.match_value" placeholder="example.com" />
            </div>
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6">
            <div class="d-flex flex-column ga-1">
              <Label for="ir-action">Action</Label>
              <v-select
                id="ir-action"
                v-model="form.action"
                :items="actionItems"
                variant="outlined"
                density="compact"
                hide-details
              />
            </div>
          </v-col>
          <v-col cols="6">
            <div class="d-flex flex-column ga-1">
              <Label for="ir-priority">Priority</Label>
              <Input id="ir-priority" v-model="form.priority" type="number" placeholder="0" />
            </div>
          </v-col>
        </v-row>
        <div class="d-flex flex-column ga-1">
          <Label for="ir-scan">Spam scan (rspamd)</Label>
          <v-select
            id="ir-scan"
            v-model="form.spam_scan"
            :items="spamScanItems"
            variant="outlined"
            density="compact"
            hide-details
          />
          <p class="text-caption text-medium-emphasis">
            <strong>default</strong> follows the global rspamd mode; <strong>off</strong> skips
            scanning; <strong>tag</strong> adds X-Spam headers; <strong>enforce</strong> rejects
            spam. Overrides apply only when an rspamd URL is configured in Settings.
          </p>
        </div>

        <!-- maildir -->
        <div v-if="form.action === 'maildir'" class="d-flex flex-column ga-1">
          <Label for="ir-maildir">Maildir Path</Label>
          <Input id="ir-maildir" v-model="form.maildir_path" placeholder="(blank = deployment base + /domain/user)" />
          <p class="text-caption text-medium-emphasis">
            Leave blank to use the global Maildir base from Settings, one mailbox per recipient.
          </p>
        </div>

        <!-- forward -->
        <template v-if="form.action === 'forward'">
          <v-row dense>
            <v-col cols="6">
              <div class="d-flex flex-column ga-1">
                <Label for="ir-fwd-host">Smarthost</Label>
                <Input id="ir-fwd-host" v-model="form.forward_host" placeholder="mail.internal" />
              </div>
            </v-col>
            <v-col cols="6">
              <div class="d-flex flex-column ga-1">
                <Label for="ir-fwd-port">Port</Label>
                <Input id="ir-fwd-port" v-model="form.forward_port" type="number" min="1" placeholder="25" />
              </div>
            </v-col>
          </v-row>
          <div class="d-flex flex-column ga-1">
            <Label for="ir-fwd-tls">TLS</Label>
            <v-select
              id="ir-fwd-tls"
              v-model="form.forward_tls"
              :items="forwardTlsItems"
              variant="outlined"
              density="compact"
              hide-details
            />
          </div>
        </template>

        <!-- webhook -->
        <template v-if="form.action === 'webhook'">
          <div class="d-flex flex-column ga-1">
            <Label for="ir-url">Destination URL (https)</Label>
            <Input id="ir-url" v-model="form.destination_url" placeholder="https://hooks.example.com/iris" />
          </div>
          <v-row dense>
            <v-col cols="6">
              <div class="d-flex flex-column ga-1">
                <Label for="ir-secret">Secret Reference</Label>
                <Input id="ir-secret" v-model="form.secret_ref" placeholder="secret://webhooks/inbound" />
                <p v-if="isEdit" class="text-caption text-medium-emphasis">Leave blank to keep the existing secret.</p>
              </div>
            </v-col>
            <v-col cols="6">
              <div class="d-flex flex-column ga-1">
                <Label for="ir-timeout">Timeout (seconds)</Label>
                <Input id="ir-timeout" v-model="form.timeout_seconds" type="number" min="1" placeholder="10" />
              </div>
            </v-col>
          </v-row>
        </template>

        <div v-if="isEdit" class="d-flex flex-column ga-1">
          <Label for="ir-status">Status</Label>
          <v-select
            id="ir-status"
            v-model="form.status"
            :items="statusItems"
            variant="outlined"
            density="compact"
            hide-details
          />
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !canSubmit">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Create' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
