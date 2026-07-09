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
import { bounceRulesService } from '@/services'
import { ApiError } from '@/services/http'
import type { BounceAction, BounceRule, TestBounceDiagnosticResult } from '@/types'

const { toast } = useToast()

const items = ref<BounceRule[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)

const search = ref('')
const classFilter = ref<'all' | 'soft' | 'hard'>('all')

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)

const emptyForm = () => ({
  smtp_code: '',
  enhanced_code: '',
  provider: '',
  pattern: '',
  class: 'soft',
  category: '',
  action: 'retry' as BounceAction,
  action_config: '',
  suggested_action: '',
  priority: 50,
  min_attempts: 0,
  suppress_ttl: '',
  status: 'active',
})
const form = ref(emptyForm())

const isEdit = computed(() => mode.value === 'edit')

const actionItems = [
  { title: 'Retry (monitor)', value: 'retry' },
  { title: 'Throttle destination', value: 'throttle' },
  { title: 'Suspend domain', value: 'suspend_domain' },
  { title: 'Suppress recipient', value: 'suppress' },
]
const classItems = [
  { title: 'Soft (transient)', value: 'soft' },
  { title: 'Hard (permanent)', value: 'hard' },
]
const providerItems = [
  { title: 'All providers', value: '' },
  { title: 'Gmail', value: 'gmail' },
  { title: 'Yahoo', value: 'yahoo' },
  { title: 'Microsoft', value: 'microsoft' },
  { title: 'Apple', value: 'apple' },
]
const statusItems = [
  { title: 'active', value: 'active' },
  { title: 'disabled', value: 'disabled' },
]

const ACTION_VARIANT: Record<BounceAction, 'default' | 'warning' | 'destructive' | 'secondary'> = {
  retry: 'default',
  throttle: 'warning',
  suspend_domain: 'destructive',
  suppress: 'secondary',
}
const ACTION_LABEL: Record<BounceAction, string> = {
  retry: 'RETRY',
  throttle: 'THROTTLE',
  suspend_domain: 'SUSPEND_DOMAIN',
  suppress: 'SUPPRESS',
}

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return items.value.filter((r) => {
    if (classFilter.value !== 'all' && r.class !== classFilter.value) return false
    if (!q) return true
    return [r.smtpCode, r.enhancedCode, r.category, r.action, r.pattern, r.provider, r.suggestedAction]
      .join(' ')
      .toLowerCase()
      .includes(q)
  })
})

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    const res = await bounceRulesService.list()
    items.value = res.items ?? []
  } catch (err) {
    if (err instanceof ApiError && err.notImplemented) notImplemented.value = true
    else if (err instanceof ApiError && err.status === 0)
      error.value = 'Cannot reach the backend. Is the API server running?'
    else error.value = err instanceof Error ? err.message : 'Failed to load bounce rules.'
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

function openEdit(r: BounceRule) {
  mode.value = 'edit'
  editId.value = r.id
  form.value = {
    smtp_code: r.smtpCode,
    enhanced_code: r.enhancedCode,
    provider: r.provider,
    pattern: r.pattern,
    class: r.class,
    category: r.category,
    action: r.action,
    action_config: r.actionConfig,
    suggested_action: r.suggestedAction,
    priority: r.priority,
    min_attempts: r.minAttempts,
    suppress_ttl: r.suppressTtl,
    status: r.status,
  }
  dialogOpen.value = true
}

async function submit() {
  saving.value = true
  try {
    const body = {
      smtp_code: form.value.smtp_code,
      enhanced_code: form.value.enhanced_code,
      provider: form.value.provider,
      pattern: form.value.pattern,
      class: form.value.class,
      category: form.value.category,
      action: form.value.action,
      action_config: form.value.action_config,
      suggested_action: form.value.suggested_action,
      priority: Number(form.value.priority) || 0,
      min_attempts: Number(form.value.min_attempts) || 0,
      suppress_ttl: form.value.suppress_ttl,
    }
    if (isEdit.value && editId.value) {
      await bounceRulesService.update(editId.value, { ...body, status: form.value.status })
      toast({ title: 'Rule updated', variant: 'success' })
    } else {
      await bounceRulesService.create(body)
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

async function remove(r: BounceRule) {
  try {
    await bounceRulesService.remove(r.id)
    toast({ title: 'Rule deleted', variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Delete failed.'
    toast({ title: 'Delete failed', description: msg, variant: 'destructive' })
  }
}

async function resetDefaults() {
  try {
    const res = await bounceRulesService.reset()
    items.value = res.items ?? []
    toast({ title: 'Defaults restored', variant: 'success' })
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Reset failed.'
    toast({ title: 'Reset failed', description: msg, variant: 'destructive' })
  }
}

// --- Test diagnostic ---
const test = ref({ smtp_code: '', domain: '', diagnostic: '', attempts: 0 })
const testResult = ref<TestBounceDiagnosticResult | null>(null)
const testing = ref(false)

async function runTest() {
  testing.value = true
  testResult.value = null
  try {
    testResult.value = await bounceRulesService.test({
      smtp_code: test.value.smtp_code,
      domain: test.value.domain,
      diagnostic: test.value.diagnostic,
      attempts: Number(test.value.attempts) || 0,
    })
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Test failed.'
    toast({ title: 'Test failed', description: msg, variant: 'destructive' })
  } finally {
    testing.value = false
  }
}

load()
</script>

<template>
  <div>
    <PageHeader
      title="Bounce Based Actions"
      description="Classify SMTP + enhanced status codes and map each failure to a system action: retry, throttle, suspend the destination, or suppress the recipient."
    >
      <template #actions>
        <Button variant="outline" :disabled="loading" @click="load">Refresh</Button>
        <Button variant="outline" @click="resetDefaults">Reset to defaults</Button>
        <Button data-testid="add-bounce-rule" @click="openCreate">Add rule</Button>
      </template>
    </PageHeader>

    <!-- Pipeline explainer -->
    <Card class="mb-4">
      <CardContent class="pa-4">
        <div class="text-overline text-medium-emphasis mb-3">
          Automated Bounce Intelligence — classification &amp; response engine
        </div>
        <v-row dense>
          <v-col cols="12" md="3">
            <div class="font-weight-medium mb-1">1. Pattern matching</div>
            <p class="text-caption text-medium-emphasis">
              Each bounce is matched against the rules by SMTP code, enhanced code, provider, and a
              diagnostic pattern.
            </p>
          </v-col>
          <v-col cols="12" md="3">
            <div class="font-weight-medium mb-1">2. Action selection</div>
            <p class="text-caption text-medium-emphasis">
              The highest-priority rule wins and picks the action: retry, throttle, suspend, or
              suppress.
            </p>
          </v-col>
          <v-col cols="12" md="3">
            <div class="font-weight-medium mb-1">3. Real-time response</div>
            <p class="text-caption text-medium-emphasis">
              Suppress adds the recipient to the suppression list; throttle/suspend compile into TSA
              back-off for the destination.
            </p>
          </v-col>
          <v-col cols="12" md="3">
            <div class="font-weight-medium mb-1">4. Reputation sync</div>
            <p class="text-caption text-medium-emphasis">
              Bounce outcomes feed the warmup and pool engines, shaping future capacity decisions.
            </p>
          </v-col>
        </v-row>
      </CardContent>
    </Card>

    <!-- Test diagnostic -->
    <Card class="mb-4">
      <CardContent class="pa-4">
        <div class="font-weight-medium mb-2">Test a diagnostic against the current rules</div>
        <v-row dense class="align-end">
          <v-col cols="12" md="2" class="d-flex flex-column ga-1">
            <Label for="t-code">SMTP code</Label>
            <Input id="t-code" v-model="test.smtp_code" placeholder="550" />
          </v-col>
          <v-col cols="12" md="3" class="d-flex flex-column ga-1">
            <Label for="t-domain">Recipient domain</Label>
            <Input id="t-domain" v-model="test.domain" placeholder="gmail.com" />
          </v-col>
          <v-col cols="12" md="4" class="d-flex flex-column ga-1">
            <Label for="t-diag">Diagnostic (SMTP response)</Label>
            <Input id="t-diag" v-model="test.diagnostic" placeholder="550 5.1.1 The email account does not exist" />
          </v-col>
          <v-col cols="6" md="1" class="d-flex flex-column ga-1">
            <Label for="t-attempts">Attempts</Label>
            <Input id="t-attempts" v-model.number="test.attempts" type="number" placeholder="0" />
          </v-col>
          <v-col cols="6" md="2">
            <Button :disabled="testing" class="w-100" @click="runTest">
              {{ testing ? 'Testing…' : 'Test' }}
            </Button>
          </v-col>
        </v-row>
        <div v-if="testResult" class="mt-3 d-flex flex-wrap align-center ga-2">
          <span class="text-caption text-medium-emphasis">Result:</span>
          <Badge variant="outline">enhanced: {{ testResult.enhancedCode || '—' }}</Badge>
          <Badge variant="outline">provider: {{ testResult.provider || 'all' }}</Badge>
          <template v-if="testResult.matched && testResult.rule">
            <span class="text-caption">→ {{ testResult.rule.category }}</span>
            <Badge :variant="ACTION_VARIANT[testResult.effectiveAction]">
              {{ ACTION_LABEL[testResult.effectiveAction] }}
            </Badge>
            <span class="text-caption text-medium-emphasis">{{ testResult.rule.suggestedAction }}</span>
          </template>
          <template v-else>
            <span class="text-caption">no rule matched →</span>
            <Badge variant="default">RETRY</Badge>
            <span class="text-caption text-medium-emphasis">(default exponential backoff)</span>
          </template>
        </div>
      </CardContent>
    </Card>

    <!-- Filters -->
    <div class="d-flex flex-wrap ga-3 mb-3">
      <Input v-model="search" placeholder="Search by code, category, or action…" class="flex-grow-1" style="min-width: 240px" />
      <v-select
        v-model="classFilter"
        :items="[{ title: 'All classes', value: 'all' }, { title: 'Soft', value: 'soft' }, { title: 'Hard', value: 'hard' }]"
        variant="outlined"
        density="compact"
        hide-details
        style="max-width: 180px"
      />
    </div>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="filtered.length === 0"
      empty-message="No bounce rules match."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Code</TableHead>
                <TableHead>Enhanced</TableHead>
                <TableHead>Category</TableHead>
                <TableHead>Class</TableHead>
                <TableHead>Provider</TableHead>
                <TableHead>Pattern</TableHead>
                <TableHead>Suggested action</TableHead>
                <TableHead>System action</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="r in filtered" :key="r.id">
                <TableCell class="font-mono font-weight-medium">{{ r.smtpCode || '—' }}</TableCell>
                <TableCell class="font-mono text-caption">{{ r.enhancedCode || '—' }}</TableCell>
                <TableCell>{{ r.category || '—' }}</TableCell>
                <TableCell>
                  <Badge :variant="r.class === 'hard' ? 'destructive' : 'warning'">{{ r.class.toUpperCase() }}</Badge>
                </TableCell>
                <TableCell>{{ r.provider || 'all' }}</TableCell>
                <TableCell class="font-mono text-caption">{{ r.pattern || '—' }}</TableCell>
                <TableCell class="text-caption text-medium-emphasis" style="max-width: 260px">
                  {{ r.suggestedAction }}
                </TableCell>
                <TableCell>
                  <div class="d-flex flex-column ga-1">
                    <Badge :variant="ACTION_VARIANT[r.action]">{{ ACTION_LABEL[r.action] }}</Badge>
                    <span v-if="r.actionConfig" class="text-caption text-medium-emphasis font-mono">{{ r.actionConfig }}</span>
                    <span v-if="r.minAttempts > 0" class="text-caption text-medium-emphasis">after {{ r.minAttempts }} tries</span>
                    <span v-if="r.action === 'suppress' && r.suppressTtl" class="text-caption text-medium-emphasis">TTL {{ r.suppressTtl }}</span>
                  </div>
                </TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end align-center ga-1">
                    <StatusBadge v-if="r.status === 'disabled'" :status="r.status" />
                    <Button variant="outline" size="sm" @click="openEdit(r)">Edit</Button>
                    <Button variant="outline" size="sm" @click="remove(r)">Delete</Button>
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
        <DialogTitle>{{ isEdit ? 'Edit bounce rule' : 'Add bounce rule' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <v-row dense>
          <v-col cols="4" class="d-flex flex-column ga-1">
            <Label for="b-code">SMTP code</Label>
            <Input id="b-code" v-model="form.smtp_code" placeholder="550 (blank = any)" />
          </v-col>
          <v-col cols="4" class="d-flex flex-column ga-1">
            <Label for="b-enh">Enhanced code</Label>
            <Input id="b-enh" v-model="form.enhanced_code" placeholder="5.1.1 (blank = any)" />
          </v-col>
          <v-col cols="4" class="d-flex flex-column ga-1">
            <Label for="b-provider">Provider</Label>
            <v-select id="b-provider" v-model="form.provider" :items="providerItems" variant="outlined" density="compact" hide-details />
          </v-col>
        </v-row>
        <div class="d-flex flex-column ga-1">
          <Label for="b-pattern">Diagnostic pattern (substring, optional)</Label>
          <Input id="b-pattern" v-model="form.pattern" placeholder="user unknown" />
        </div>
        <v-row dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="b-category">Category</Label>
            <Input id="b-category" v-model="form.category" placeholder="Invalid Recipient" />
          </v-col>
          <v-col cols="3" class="d-flex flex-column ga-1">
            <Label for="b-class">Class</Label>
            <v-select id="b-class" v-model="form.class" :items="classItems" variant="outlined" density="compact" hide-details />
          </v-col>
          <v-col cols="3" class="d-flex flex-column ga-1">
            <Label for="b-priority">Priority</Label>
            <Input id="b-priority" v-model.number="form.priority" type="number" placeholder="100" />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="b-action">System action</Label>
            <v-select id="b-action" v-model="form.action" :items="actionItems" variant="outlined" density="compact" hide-details />
          </v-col>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="b-cfg">Action config (throttle: name=value · suspend: duration)</Label>
            <Input id="b-cfg" v-model="form.action_config" placeholder="max_message_rate=100/h or 2h" />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="b-min">Min attempts (apply only after N tries)</Label>
            <Input id="b-min" v-model.number="form.min_attempts" type="number" placeholder="0" />
          </v-col>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="b-ttl">Suppress TTL (blank = global default)</Label>
            <Input id="b-ttl" v-model="form.suppress_ttl" placeholder="30d" :disabled="form.action !== 'suppress'" />
          </v-col>
        </v-row>
        <div class="d-flex flex-column ga-1">
          <Label for="b-suggested">Suggested action (operator guidance)</Label>
          <Input id="b-suggested" v-model="form.suggested_action" placeholder="Recipient does not exist; suppress the address." />
        </div>
        <div v-if="isEdit" class="d-flex flex-column ga-1">
          <Label for="b-status">Status</Label>
          <v-select id="b-status" v-model="form.status" :items="statusItems" variant="outlined" density="compact" hide-details />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !form.action">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Add' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
