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

// v-select item lists ({ title, value }) for the dropdowns.
const actionItems = [
  { title: 'suspend', value: 'suspend' },
  { title: 'suspend tenant', value: 'suspend_tenant' },
  { title: 'set config (tighten a limit)', value: 'set_config' },
]
const configNameItems = [
  { title: 'max_message_rate', value: 'max_message_rate' },
  { title: 'max_connection_rate', value: 'max_connection_rate' },
  { title: 'connection_limit', value: 'connection_limit' },
  { title: 'max_deliveries_per_connection', value: 'max_deliveries_per_connection' },
]
const statusItems = [
  { title: 'active', value: 'active' },
  { title: 'disabled', value: 'disabled' },
]

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
        <CardContent class="pa-0">
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
                <TableCell class="font-mono text-caption">{{ r.domain }}</TableCell>
                <TableCell class="font-mono text-caption">{{ r.regex }}</TableCell>
                <TableCell>{{ actionLabel(r) }}</TableCell>
                <TableCell>{{ r.trigger }}</TableCell>
                <TableCell>{{ r.duration }}</TableCell>
                <TableCell><StatusBadge :status="r.status" /></TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-1">
                    <Button variant="outline" size="sm" @click="openEdit(r)">Edit</Button>
                    <Button variant="outline" size="sm" @click="toggle(r)">
                      {{ r.status === 'active' ? 'Disable' : 'Enable' }}
                    </Button>
                  </div>
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
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <v-row dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="a-domain">Domain</Label>
            <Input id="a-domain" v-model="form.domain" placeholder="comcast.net or default" />
          </v-col>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="a-duration">Duration</Label>
            <Input id="a-duration" v-model="form.duration" placeholder="2 hours" />
          </v-col>
        </v-row>
        <div class="d-flex flex-column ga-1">
          <Label for="a-regex">Match (regex on the SMTP response)</Label>
          <Input id="a-regex" v-model="form.regex" placeholder="RL0000" />
        </div>
        <v-row dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="a-action">Action</Label>
            <v-select
              id="a-action"
              v-model="form.action"
              :items="actionItems"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="a-trigger">Trigger</Label>
            <Input id="a-trigger" v-model="form.trigger" placeholder="immediate or 2/hr" />
          </v-col>
        </v-row>
        <v-row v-if="isSetConfig" dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="a-cfg-name">Limit</Label>
            <v-select
              id="a-cfg-name"
              v-model="form.config_name"
              :items="configNameItems"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="a-cfg-value">Value</Label>
            <Input id="a-cfg-value" v-model="form.config_value" placeholder="100/h" />
          </v-col>
        </v-row>
        <div v-if="isEdit" class="d-flex flex-column ga-1">
          <Label for="a-status">Status</Label>
          <v-select
            id="a-status"
            v-model="form.status"
            :items="statusItems"
            variant="outlined"
            density="compact"
            hide-details
          />
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
