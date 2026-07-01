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
import { useToast } from '@/composables/useToast'
import { automationService } from '@/services'
import { ApiError } from '@/services/http'
import type { AutomationRule } from '@/types'

const { toast } = useToast()

const items = ref<AutomationRule[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref({
  domain: '',
  regex: '',
  action: 'suspend',
  config_name: 'max_message_rate',
  config_value: '',
  trigger: 'immediate',
  duration: '1 hour',
  status: 'active',
})

const isEdit = computed(() => mode.value === 'edit')
const isSetConfig = computed(() => form.value.action === 'set_config')

function actionLabel(r: AutomationRule): string {
  if (r.action === 'set_config') return `set ${r.configName}=${r.configValue}`
  if (r.action === 'suspend_tenant') return 'suspend tenant'
  return 'suspend'
}

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    const res = await automationService.list()
    items.value = res.items ?? []
  } catch (err) {
    if (err instanceof ApiError && err.notImplemented) notImplemented.value = true
    else if (err instanceof ApiError && err.status === 0)
      error.value = 'Cannot reach the backend. Is the API server running?'
    else error.value = err instanceof Error ? err.message : 'Failed to load automation rules.'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = {
    domain: '',
    regex: '',
    action: 'suspend',
    config_name: 'max_message_rate',
    config_value: '',
    trigger: 'immediate',
    duration: '1 hour',
    status: 'active',
  }
  dialogOpen.value = true
}

function openEdit(r: AutomationRule) {
  mode.value = 'edit'
  editId.value = r.id
  form.value = {
    domain: r.domain,
    regex: r.regex,
    action: r.action,
    config_name: r.configName || 'max_message_rate',
    config_value: r.configValue || '',
    trigger: r.trigger,
    duration: r.duration,
    status: r.status,
  }
  dialogOpen.value = true
}

async function submit() {
  if (!form.value.domain || !form.value.regex) return
  saving.value = true
  try {
    const body = {
      domain: form.value.domain,
      regex: form.value.regex,
      action: form.value.action,
      config_name: isSetConfig.value ? form.value.config_name : '',
      config_value: isSetConfig.value ? form.value.config_value : '',
      trigger: form.value.trigger,
      duration: form.value.duration,
    }
    if (isEdit.value && editId.value) {
      await automationService.update(editId.value, { ...body, status: form.value.status })
      toast({ title: 'Rule updated', variant: 'success' })
    } else {
      await automationService.create(body)
      toast({ title: 'Rule added', variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save rule.'
    toast({ title: 'Save failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

async function toggle(r: AutomationRule) {
  try {
    await automationService.setStatus(r.id, r.status === 'active' ? 'disabled' : 'active')
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Toggle failed.'
    toast({ title: 'Toggle failed', description: msg, variant: 'destructive' })
  }
}

load()
</script>

<template>
  <div>
    <PageHeader
      title="Shaping Automation"
      description="Reactive back-off rules: when a destination's SMTP response matches, suspend or tighten a limit. Evaluated by the TSA daemon under the IP-warmup ceiling."
    >
      <template #actions>
        <Button variant="outline" :disabled="loading" @click="load">Refresh</Button>
        <Button data-testid="add-automation" @click="openCreate">Add Rule</Button>
      </template>
    </PageHeader>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No automation rules yet."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead>Match (regex)</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Trigger</TableHead>
                <TableHead>Duration</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="r in items" :key="r.id">
                <TableCell class="font-mono text-xs">{{ r.domain }}</TableCell>
                <TableCell class="font-mono text-xs">{{ r.regex }}</TableCell>
                <TableCell>{{ actionLabel(r) }}</TableCell>
                <TableCell>{{ r.trigger }}</TableCell>
                <TableCell>{{ r.duration }}</TableCell>
                <TableCell><StatusBadge :status="r.status" /></TableCell>
                <TableCell class="space-x-1 text-right">
                  <Button variant="outline" size="sm" @click="openEdit(r)">Edit</Button>
                  <Button variant="outline" size="sm" @click="toggle(r)">
                    {{ r.status === 'active' ? 'Disable' : 'Enable' }}
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
        <DialogTitle>{{ isEdit ? 'Edit rule' : 'Add automation rule' }}</DialogTitle>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="a-domain">Domain</Label>
            <Input id="a-domain" v-model="form.domain" placeholder="comcast.net or default" />
          </div>
          <div class="space-y-1.5">
            <Label for="a-duration">Duration</Label>
            <Input id="a-duration" v-model="form.duration" placeholder="2 hours" />
          </div>
        </div>
        <div class="space-y-1.5">
          <Label for="a-regex">Match (regex on the SMTP response)</Label>
          <Input id="a-regex" v-model="form.regex" placeholder="RL0000" />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="a-action">Action</Label>
            <Select id="a-action" v-model="form.action">
              <option value="suspend">suspend</option>
              <option value="suspend_tenant">suspend tenant</option>
              <option value="set_config">set config (tighten a limit)</option>
            </Select>
          </div>
          <div class="space-y-1.5">
            <Label for="a-trigger">Trigger</Label>
            <Input id="a-trigger" v-model="form.trigger" placeholder="immediate or 2/hr" />
          </div>
        </div>
        <div v-if="isSetConfig" class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="a-cfg-name">Limit</Label>
            <Select id="a-cfg-name" v-model="form.config_name">
              <option value="max_message_rate">max_message_rate</option>
              <option value="max_connection_rate">max_connection_rate</option>
              <option value="connection_limit">connection_limit</option>
              <option value="max_deliveries_per_connection">max_deliveries_per_connection</option>
            </Select>
          </div>
          <div class="space-y-1.5">
            <Label for="a-cfg-value">Value</Label>
            <Input id="a-cfg-value" v-model="form.config_value" placeholder="100/h" />
          </div>
        </div>
        <div v-if="isEdit" class="space-y-1.5">
          <Label for="a-status">Status</Label>
          <Select id="a-status" v-model="form.status">
            <option value="active">active</option>
            <option value="disabled">disabled</option>
          </Select>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !form.domain || !form.regex">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Add' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
