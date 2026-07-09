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
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useToast } from '@/composables/useToast'
import { eventProcessorsService } from '@/services'
import { ApiError } from '@/services/http'
import type { EventProcessor } from '@/types'

const { toast } = useToast()

const items = ref<EventProcessor[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)

const dialogOpen = ref(false)
const saving = ref(false)
const testing = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)

const EVENT_TYPES = [
  { value: 'bounce', title: 'Bounce' },
  { value: 'suppression_created', title: 'Suppression created' },
  { value: 'feedback_report', title: 'Feedback report' },
  { value: 'dmarc_received', title: 'DMARC received' },
]
const driverItems = [
  { title: 'Webhook (HTTP POST)', value: 'webhook' },
  { title: 'Redis (XADD stream)', value: 'redis' },
  { title: 'GreenArrow (Event Notification)', value: 'greenarrow' },
]
const modeItems = [
  { title: 'Single (one delivery per event)', value: 'single' },
  { title: 'Batch (accumulate)', value: 'batch' },
]
const formatItems = [
  { title: 'Native (iris format)', value: 'native' },
  { title: 'GreenArrow bounce_all', value: 'greenarrow_bounce_all' },
]
const statusItems = [
  { title: 'active', value: 'active' },
  { title: 'disabled', value: 'disabled' },
]

const emptyForm = () => ({
  name: '',
  event_types: [] as string[],
  mailclasses: '' as string, // comma/space separated in the form
  driver: 'webhook',
  format: 'native',
  // webhook fields
  url: '',
  secret: '',
  headers: '',
  timeout: '',
  // redis fields
  addr: '',
  stream: '',
  password: '',
  db: '',
  // greenarrow fields
  max_batch_size: 20,
  mode: 'single',
  batch_max_size: 100,
  batch_max_wait: '5s',
  status: 'active',
})
const form = ref(emptyForm())

const isEdit = computed(() => mode.value === 'edit')
const isWebhook = computed(() => form.value.driver === 'webhook')
const isGreenArrow = computed(() => form.value.driver === 'greenarrow')
const isBatch = computed(() => form.value.mode === 'batch')

function parseList(s: string): string[] {
  return s.split(/[\s,]+/).map((v) => v.trim()).filter(Boolean)
}

function driverConfig(): Record<string, string> {
  if (isGreenArrow.value) {
    // GreenArrow uses its own fixed wire format; no `format` key applies.
    return {
      url: form.value.url,
      max_batch_size: String(form.value.max_batch_size || 20),
      headers: form.value.headers,
      timeout: form.value.timeout,
    }
  }
  const base = { format: form.value.format }
  if (isWebhook.value) {
    return { ...base, url: form.value.url, secret: form.value.secret, headers: form.value.headers, timeout: form.value.timeout }
  }
  return { ...base, addr: form.value.addr, stream: form.value.stream, password: form.value.password, db: form.value.db }
}

function requestBody() {
  return {
    name: form.value.name,
    event_types: form.value.event_types,
    mailclasses: parseList(form.value.mailclasses),
    driver: form.value.driver,
    driver_config: driverConfig(),
    mode: form.value.mode,
    batch_max_size: Number(form.value.batch_max_size) || 0,
    batch_max_wait: form.value.batch_max_wait,
  }
}

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    const res = await eventProcessorsService.list()
    items.value = res.items ?? []
  } catch (err) {
    if (err instanceof ApiError && err.notImplemented) notImplemented.value = true
    else if (err instanceof ApiError && err.status === 0)
      error.value = 'Cannot reach the backend. Is the API server running?'
    else error.value = err instanceof Error ? err.message : 'Failed to load event processors.'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = emptyForm()
  dialogOpen.value = true
}

function openEdit(p: EventProcessor) {
  mode.value = 'edit'
  editId.value = p.id
  const c = p.driverConfig || {}
  form.value = {
    ...emptyForm(),
    name: p.name,
    event_types: [...p.eventTypes],
    mailclasses: p.mailclasses.join(', '),
    driver: p.driver,
    format: c.format || 'native',
    url: c.url || '',
    secret: c.secret || '',
    headers: c.headers || '',
    timeout: c.timeout || '',
    addr: c.addr || '',
    stream: c.stream || '',
    password: c.password || '',
    db: c.db || '',
    max_batch_size: Number(c.max_batch_size) || 20,
    mode: p.mode,
    batch_max_size: p.batchMaxSize || 100,
    batch_max_wait: p.batchMaxWait || '',
    status: p.status,
  }
  dialogOpen.value = true
}

async function submit() {
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await eventProcessorsService.update(editId.value, { ...requestBody(), status: form.value.status })
      toast({ title: 'Processor updated', variant: 'success' })
    } else {
      await eventProcessorsService.create(requestBody())
      toast({ title: 'Processor added', variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save processor.'
    toast({ title: 'Save failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

async function sendTest() {
  testing.value = true
  try {
    const res = await eventProcessorsService.test(requestBody())
    if (res.ok) toast({ title: 'Test event delivered', variant: 'success' })
    else toast({ title: 'Test delivery failed', description: res.error, variant: 'destructive' })
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Test failed.'
    toast({ title: 'Test failed', description: msg, variant: 'destructive' })
  } finally {
    testing.value = false
  }
}

async function remove(p: EventProcessor) {
  try {
    await eventProcessorsService.remove(p.id)
    toast({ title: 'Processor deleted', variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Delete failed.'
    toast({ title: 'Delete failed', description: msg, variant: 'destructive' })
  }
}

function destinationLabel(p: EventProcessor): string {
  const c = p.driverConfig || {}
  if (p.driver === 'webhook' || p.driver === 'greenarrow') return c.url || '(no url)'
  if (p.driver === 'redis') return `${c.addr || 'iris redis'} → ${c.stream || '(no stream)'}`
  return p.driver
}

load()
</script>

<template>
  <div>
    <PageHeader
      title="Event Processors"
      description="Forward internal events (bounce, suppression, feedback, DMARC) to external services via a pluggable driver — webhook or redis — filtered by event type and mailclass, one-per-event or in batches."
    >
      <template #actions>
        <Button variant="outline" :disabled="loading" @click="load">Refresh</Button>
        <Button data-testid="add-event-processor" @click="openCreate">Add processor</Button>
      </template>
    </PageHeader>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No event processors yet."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Events</TableHead>
                <TableHead>Mailclasses</TableHead>
                <TableHead>Driver</TableHead>
                <TableHead>Destination</TableHead>
                <TableHead>Mode</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="p in items" :key="p.id">
                <TableCell class="font-weight-medium">{{ p.name }}</TableCell>
                <TableCell>
                  <div class="d-flex flex-wrap ga-1">
                    <Badge v-for="t in p.eventTypes" :key="t" variant="secondary">{{ t }}</Badge>
                  </div>
                </TableCell>
                <TableCell>
                  <span v-if="p.mailclasses.length === 0" class="text-medium-emphasis">all</span>
                  <div v-else class="d-flex flex-wrap ga-1">
                    <Badge v-for="m in p.mailclasses" :key="m" variant="outline">{{ m }}</Badge>
                  </div>
                </TableCell>
                <TableCell><Badge variant="default">{{ p.driver }}</Badge></TableCell>
                <TableCell class="font-mono text-caption text-break" style="max-width: 260px">{{ destinationLabel(p) }}</TableCell>
                <TableCell>
                  {{ p.mode }}
                  <span v-if="p.mode === 'batch'" class="text-caption text-medium-emphasis">
                    ({{ p.batchMaxSize }}/{{ p.batchMaxWait || '—' }})
                  </span>
                </TableCell>
                <TableCell><StatusBadge :status="p.status" /></TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-1">
                    <Button variant="outline" size="sm" @click="openEdit(p)">Edit</Button>
                    <Button variant="outline" size="sm" @click="remove(p)">Delete</Button>
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
        <DialogTitle>{{ isEdit ? 'Edit event processor' : 'Add event processor' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="ep-name">Name</Label>
          <Input id="ep-name" v-model="form.name" placeholder="Notify billing system" />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label>Event types</Label>
          <v-select
            v-model="form.event_types"
            :items="EVENT_TYPES"
            multiple
            chips
            closable-chips
            variant="outlined"
            density="compact"
            hide-details
          />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="ep-classes">Mailclasses (comma-separated; blank = all)</Label>
          <Input id="ep-classes" v-model="form.mailclasses" placeholder="acme_s, homesbg_h" />
        </div>
        <v-row dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="ep-driver">Driver</Label>
            <v-select id="ep-driver" v-model="form.driver" :items="driverItems" variant="outlined" density="compact" hide-details />
          </v-col>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="ep-mode">Mode</Label>
            <v-select id="ep-mode" v-model="form.mode" :items="modeItems" variant="outlined" density="compact" hide-details />
          </v-col>
        </v-row>
        <div v-if="!isGreenArrow" class="d-flex flex-column ga-1">
          <Label for="ep-format">Payload format</Label>
          <v-select id="ep-format" v-model="form.format" :items="formatItems" variant="outlined" density="compact" hide-details />
          <p class="text-caption text-medium-emphasis">
            GreenArrow bounce_all reshapes bounce events to GreenArrow Engine's schema (non-bounce
            events stay native).
          </p>
        </div>

        <!-- Webhook config -->
        <template v-if="isWebhook">
          <div class="d-flex flex-column ga-1">
            <Label for="ep-url">Webhook URL</Label>
            <Input id="ep-url" v-model="form.url" placeholder="https://hooks.example.com/iris" />
          </div>
          <v-row dense>
            <v-col cols="6" class="d-flex flex-column ga-1">
              <Label for="ep-secret">HMAC secret (optional)</Label>
              <Input id="ep-secret" v-model="form.secret" placeholder="signs X-Iris-Signature" />
            </v-col>
            <v-col cols="6" class="d-flex flex-column ga-1">
              <Label for="ep-timeout">Timeout (optional)</Label>
              <Input id="ep-timeout" v-model="form.timeout" placeholder="10s" />
            </v-col>
          </v-row>
          <div class="d-flex flex-column ga-1">
            <Label for="ep-headers">Extra headers (one "Key: Value" per line)</Label>
            <Input id="ep-headers" v-model="form.headers" placeholder="Authorization: Bearer …" />
          </div>
        </template>

        <!-- GreenArrow config -->
        <template v-else-if="isGreenArrow">
          <v-alert type="info" variant="tonal" density="compact" class="text-caption">
            Posts a bare JSON array in GreenArrow's Event-Notification format so an existing
            <code>ga_handler</code> endpoint works unchanged. Emits <code>bounce_all</code>,
            <code>bounce_bad_address</code> (bad-address hard bounces) and <code>scomp</code> (from
            feedback reports) — so pick only <strong>Bounce</strong> and/or
            <strong>Feedback report</strong> above. The receiving IP must allow iris's egress IP.
          </v-alert>
          <div class="d-flex flex-column ga-1">
            <Label for="ep-ga-url">Endpoint URL</Label>
            <Input id="ep-ga-url" v-model="form.url" placeholder="https://www.example.com/ga_handler.php" />
          </div>
          <v-row dense>
            <v-col cols="6" class="d-flex flex-column ga-1">
              <Label for="ep-ga-batch">Max events per POST</Label>
              <Input id="ep-ga-batch" v-model.number="form.max_batch_size" type="number" placeholder="20" />
            </v-col>
            <v-col cols="6" class="d-flex flex-column ga-1">
              <Label for="ep-ga-timeout">Timeout (optional)</Label>
              <Input id="ep-ga-timeout" v-model="form.timeout" placeholder="10s" />
            </v-col>
          </v-row>
          <div class="d-flex flex-column ga-1">
            <Label for="ep-ga-headers">Extra headers (one "Key: Value" per line)</Label>
            <Input id="ep-ga-headers" v-model="form.headers" placeholder="Authorization: Bearer …" />
          </div>
        </template>

        <!-- Redis config -->
        <template v-else>
          <v-row dense>
            <v-col cols="6" class="d-flex flex-column ga-1">
              <Label for="ep-stream">Stream</Label>
              <Input id="ep-stream" v-model="form.stream" placeholder="iris:events" />
            </v-col>
            <v-col cols="6" class="d-flex flex-column ga-1">
              <Label for="ep-addr">Redis addr (blank = iris redis)</Label>
              <Input id="ep-addr" v-model="form.addr" placeholder="redis:6379" />
            </v-col>
          </v-row>
          <v-row dense>
            <v-col cols="6" class="d-flex flex-column ga-1">
              <Label for="ep-pass">Password (optional)</Label>
              <Input id="ep-pass" v-model="form.password" />
            </v-col>
            <v-col cols="6" class="d-flex flex-column ga-1">
              <Label for="ep-db">DB (optional)</Label>
              <Input id="ep-db" v-model="form.db" placeholder="0" />
            </v-col>
          </v-row>
        </template>

        <v-row v-if="isBatch" dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="ep-bsize">Batch max size</Label>
            <Input id="ep-bsize" v-model.number="form.batch_max_size" type="number" placeholder="100" />
          </v-col>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="ep-bwait">Batch max wait</Label>
            <Input id="ep-bwait" v-model="form.batch_max_wait" placeholder="5s" />
          </v-col>
        </v-row>

        <div v-if="isEdit" class="d-flex flex-column ga-1">
          <Label for="ep-status">Status</Label>
          <v-select id="ep-status" v-model="form.status" :items="statusItems" variant="outlined" density="compact" hide-details />
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="button" variant="outline" :disabled="testing" @click="sendTest">
            {{ testing ? 'Testing…' : 'Send test event' }}
          </Button>
          <Button type="submit" :disabled="saving || !form.name || form.event_types.length === 0">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Add' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
