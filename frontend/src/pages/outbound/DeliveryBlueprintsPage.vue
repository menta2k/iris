<script setup lang="ts">
import { computed, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
import { blueprintsService } from '@/services'
import { ApiError } from '@/services/http'
import type { DeliveryBlueprint } from '@/types'

const { toast } = useToast()

const items = ref<DeliveryBlueprint[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)
const seeding = ref(false)

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref({
  provider: '',
  mx_pattern: '',
  conn_rate: '5/min',
  deliveries_per_conn: 10,
  conn_limit: 3,
  daily_cap: 150,
  status: 'active',
})

const isEdit = computed(() => mode.value === 'edit')

// Group blueprints by provider, preserving a stable provider order.
const grouped = computed(() => {
  const map = new Map<string, DeliveryBlueprint[]>()
  for (const b of items.value) {
    const arr = map.get(b.provider) ?? []
    arr.push(b)
    map.set(b.provider, arr)
  }
  return [...map.entries()].map(([provider, rules]) => ({ provider, rules }))
})

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    const res = await blueprintsService.list()
    items.value = res.items ?? []
  } catch (err) {
    if (err instanceof ApiError && err.notImplemented) notImplemented.value = true
    else if (err instanceof ApiError && err.status === 0)
      error.value = 'Cannot reach the backend. Is the API server running?'
    else error.value = err instanceof Error ? err.message : 'Failed to load blueprints.'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = {
    provider: '',
    mx_pattern: '',
    conn_rate: '5/min',
    deliveries_per_conn: 10,
    conn_limit: 3,
    daily_cap: 150,
    status: 'active',
  }
  dialogOpen.value = true
}

function openEdit(b: DeliveryBlueprint) {
  mode.value = 'edit'
  editId.value = b.id
  form.value = {
    provider: b.provider,
    mx_pattern: b.mxPattern,
    conn_rate: b.connRate,
    deliveries_per_conn: b.deliveriesPerConn,
    conn_limit: b.connLimit,
    daily_cap: b.dailyCap,
    status: b.status,
  }
  dialogOpen.value = true
}

async function submit() {
  if (!form.value.provider || !form.value.mx_pattern) return
  saving.value = true
  try {
    const body = {
      provider: form.value.provider,
      mx_pattern: form.value.mx_pattern,
      conn_rate: form.value.conn_rate,
      deliveries_per_conn: Number(form.value.deliveries_per_conn),
      conn_limit: Number(form.value.conn_limit),
      daily_cap: Number(form.value.daily_cap),
    }
    if (isEdit.value && editId.value) {
      await blueprintsService.update(editId.value, { ...body, status: form.value.status })
      toast({ title: 'Blueprint updated', description: form.value.mx_pattern, variant: 'success' })
    } else {
      await blueprintsService.create(body)
      toast({ title: 'Blueprint added', description: form.value.mx_pattern, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save blueprint.'
    toast({ title: 'Save failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

async function toggle(b: DeliveryBlueprint) {
  try {
    await blueprintsService.setStatus(b.id, b.status === 'active' ? 'disabled' : 'active')
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Toggle failed.'
    toast({ title: 'Toggle failed', description: msg, variant: 'destructive' })
  }
}

async function seedDefaults() {
  seeding.value = true
  try {
    const res = await blueprintsService.seedDefaults()
    toast({
      title: 'Defaults seeded',
      description: `${res.inserted ?? 0} provider rule(s) added.`,
      variant: 'success',
    })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Seed failed.'
    toast({ title: 'Seed failed', description: msg, variant: 'destructive' })
  } finally {
    seeding.value = false
  }
}

load()
</script>

<template>
  <div>
    <PageHeader
      title="Global Delivery Blueprints"
      description="Default fallbacks and blueprints. These limits are the starting point for new IPs; real-time limits are managed per-IP by the warmup engine and adaptive throttling."
    >
      <template #actions>
        <Button variant="outline" :disabled="loading" @click="load">Refresh</Button>
        <Button variant="outline" :disabled="seeding" @click="seedDefaults">
          {{ seeding ? 'Seeding…' : 'Seed Defaults' }}
        </Button>
        <Button data-testid="add-blueprint" @click="openCreate">Add Rule</Button>
      </template>
    </PageHeader>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No blueprints yet. Use “Seed Defaults” to import the major providers."
    >
      <div class="space-y-6">
        <Card v-for="g in grouped" :key="g.provider">
          <CardHeader class="flex-row items-center gap-2 space-y-0">
            <CardTitle>{{ g.provider }}</CardTitle>
            <span class="text-xs text-muted-foreground">{{ g.rules.length }} rules</span>
          </CardHeader>
          <CardContent class="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>MX Pattern</TableHead>
                  <TableHead>Conn Rate</TableHead>
                  <TableHead>Deliveries/Conn</TableHead>
                  <TableHead>Conn Limit (default)</TableHead>
                  <TableHead>Daily Cap (default)</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead class="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow v-for="b in g.rules" :key="b.id">
                  <TableCell class="font-mono text-xs">{{ b.mxPattern }}</TableCell>
                  <TableCell>{{ b.connRate || '—' }}</TableCell>
                  <TableCell class="tabular-nums">{{ b.deliveriesPerConn }}</TableCell>
                  <TableCell class="tabular-nums">{{ b.connLimit }}</TableCell>
                  <TableCell class="tabular-nums">{{ b.dailyCap.toLocaleString() }}</TableCell>
                  <TableCell><StatusBadge :status="b.status" /></TableCell>
                  <TableCell class="space-x-1 text-right">
                    <Button variant="outline" size="sm" @click="openEdit(b)">Edit</Button>
                    <Button variant="outline" size="sm" @click="toggle(b)">
                      {{ b.status === 'active' ? 'Disable' : 'Enable' }}
                    </Button>
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </DataState>

    <Dialog v-model:open="dialogOpen">
      <DialogHeader>
        <DialogTitle>{{ isEdit ? 'Edit blueprint' : 'Add blueprint' }}</DialogTitle>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="bp-provider">Provider</Label>
            <Input id="bp-provider" v-model="form.provider" placeholder="Gmail" />
          </div>
          <div class="space-y-1.5">
            <Label for="bp-mx">MX Pattern</Label>
            <Input id="bp-mx" v-model="form.mx_pattern" placeholder="google.com" />
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="bp-rate">Conn Rate</Label>
            <Input id="bp-rate" v-model="form.conn_rate" placeholder="5/min" />
          </div>
          <div class="space-y-1.5">
            <Label for="bp-deliveries">Deliveries / Conn</Label>
            <Input id="bp-deliveries" v-model.number="form.deliveries_per_conn" type="number" />
          </div>
          <div class="space-y-1.5">
            <Label for="bp-conn-limit">Conn Limit</Label>
            <Input id="bp-conn-limit" v-model.number="form.conn_limit" type="number" />
          </div>
          <div class="space-y-1.5">
            <Label for="bp-daily">Daily Cap</Label>
            <Input id="bp-daily" v-model.number="form.daily_cap" type="number" />
          </div>
        </div>
        <div v-if="isEdit" class="space-y-1.5">
          <Label for="bp-status">Status</Label>
          <Select id="bp-status" v-model="form.status">
            <option value="active">active</option>
            <option value="disabled">disabled</option>
          </Select>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !form.provider || !form.mx_pattern">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Add' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
