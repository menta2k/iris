<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
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
import { Select } from '@/components/ui/select'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { outboundConfigService } from '@/services'
import { ApiError } from '@/services/http'
import type { RoutingRule, VMTA, VMTAGroup } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<RoutingRule>({
  loader: () => outboundConfigService.listRoutingRules(),
})
const { toast } = useToast()

// Targets the rule can point at, loaded so the Target field is a dropdown and
// the table can show names instead of raw ids.
const availableVmtas = ref<VMTA[]>([])
const availableGroups = ref<VMTAGroup[]>([])

async function loadTargets() {
  const [v, g] = await Promise.allSettled([
    outboundConfigService.listVmtas(),
    outboundConfigService.listVmtaGroups(),
  ])
  availableVmtas.value = v.status === 'fulfilled' ? (v.value.items ?? []) : []
  availableGroups.value = g.status === 'fulfilled' ? (g.value.items ?? []) : []
  ensureTargetSelected()
}

// Default the Target dropdown to the first available option when nothing is
// chosen yet, so a freshly opened create dialog is valid once name + value are
// filled (an existing edit selection is preserved).
function ensureTargetSelected() {
  if (!form.value.target_id && targetOptions.value.length) {
    form.value.target_id = targetOptions.value[0].id
  }
}

// Values are the backend enum; labels are operator-friendly.
const matchTypes = [
  { value: 'mailclass', label: 'Mail Class (header + value)' },
  { value: 'recipient_email', label: 'Recipient Email' },
  { value: 'recipient_domain', label: 'Recipient Domain' },
]
const targetTypes = [
  { value: 'vmta', label: 'VMTA' },
  { value: 'vmta_group', label: 'VMTA Group' },
]
const RULE_STATUSES = ['active', 'disabled']

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref({
  name: '',
  match_type: 'recipient_domain',
  match_header: 'X-Mail-Class',
  match_value: '',
  priority: 100,
  target_type: 'vmta_group',
  target_id: '',
  status: 'active',
})

const isEdit = computed(() => mode.value === 'edit')
const isMailclass = computed(() => form.value.match_type === 'mailclass')

// Options for the Target dropdown follow the selected target type.
const targetOptions = computed(() =>
  form.value.target_type === 'vmta'
    ? availableVmtas.value.map((v) => ({ id: v.id, label: `${v.name} (${v.ipAddress})` }))
    : availableGroups.value.map((g) => ({ id: g.id, label: g.name })),
)

// When the target type changes, drop a now-invalid selection and pick the first
// valid option of the new type.
function onTargetTypeChange() {
  if (!targetOptions.value.some((o) => o.id === form.value.target_id)) {
    form.value.target_id = ''
  }
  ensureTargetSelected()
}

// Resolve a rule's target id to a human name for the table.
function targetName(r: RoutingRule): string {
  if (r.targetType === 'vmta') {
    return availableVmtas.value.find((v) => v.id === r.targetId)?.name ?? r.targetId
  }
  return availableGroups.value.find((g) => g.id === r.targetId)?.name ?? r.targetId
}

onMounted(loadTargets)

async function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = {
    name: '',
    match_type: 'recipient_domain',
    match_header: 'X-Mail-Class',
    match_value: '',
    priority: 100,
    target_type: 'vmta_group',
    target_id: '',
    status: 'active',
  }
  dialogOpen.value = true
  await loadTargets()
}

async function openEdit(r: RoutingRule) {
  mode.value = 'edit'
  editId.value = r.id
  form.value = {
    name: r.name,
    match_type: r.matchType,
    match_header: r.matchHeader || 'X-Mail-Class',
    match_value: r.matchValue,
    priority: r.priority,
    target_type: r.targetType,
    target_id: r.targetId,
    status: (r.status || 'active').toLowerCase(),
  }
  dialogOpen.value = true
  await loadTargets()
}

async function submit() {
  if (!form.value.name || !form.value.match_value || !form.value.target_id) return
  saving.value = true
  try {
    // match_header only applies to mailclass matches.
    const matchHeader = form.value.match_type === 'mailclass' ? form.value.match_header : ''
    if (isEdit.value && editId.value) {
      await outboundConfigService.updateRoutingRule(editId.value, {
        ...form.value,
        match_header: matchHeader,
        priority: Number(form.value.priority),
      })
      toast({ title: 'Routing rule updated', description: form.value.name, variant: 'success' })
    } else {
      await outboundConfigService.createRoutingRule({
        name: form.value.name,
        match_type: form.value.match_type,
        match_header: matchHeader,
        match_value: form.value.match_value,
        priority: Number(form.value.priority),
        target_type: form.value.target_type,
        target_id: form.value.target_id,
      })
      toast({ title: 'Routing rule created', description: form.value.name, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save rule.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="Routing Rules" description="Match conditions that route mail to VMTAs or groups.">
      <template #actions>
        <Button data-testid="create-routing-rule" @click="openCreate">New Rule</Button>
      </template>
    </PageHeader>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No routing rules configured yet."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Priority</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Match</TableHead>
                <TableHead>Target</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="r in items" :key="r.id">
                <TableCell class="tabular-nums">{{ r.priority }}</TableCell>
                <TableCell class="font-medium">{{ r.name }}</TableCell>
                <TableCell>
                  <Badge variant="outline">{{ r.matchType }}</Badge>
                  <span class="ml-2 font-mono text-xs">
                    <template v-if="r.matchType === 'mailclass'">{{ r.matchHeader }}: </template>{{ r.matchValue }}
                  </span>
                </TableCell>
                <TableCell>
                  <Badge variant="secondary">{{ r.targetType }}</Badge>
                  <span class="ml-2 text-xs">{{ targetName(r) }}</span>
                </TableCell>
                <TableCell><StatusBadge :status="r.status" /></TableCell>
                <TableCell class="text-right">
                  <Button
                    variant="outline"
                    size="sm"
                    :data-testid="`edit-routing-rule-${r.id}`"
                    @click="openEdit(r)"
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
        <DialogTitle>{{ isEdit ? 'Edit Routing Rule' : 'Create Routing Rule' }}</DialogTitle>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="space-y-1.5">
          <Label for="rr-name">Name</Label>
          <Input id="rr-name" v-model="form.name" placeholder="route-gmail" />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="rr-match-type">Match Type</Label>
            <Select id="rr-match-type" v-model="form.match_type">
              <option v-for="t in matchTypes" :key="t.value" :value="t.value">{{ t.label }}</option>
            </Select>
          </div>
          <div class="space-y-1.5">
            <Label for="rr-match-value">{{ isMailclass ? 'Header Value' : 'Match Value' }}</Label>
            <Input
              id="rr-match-value"
              v-model="form.match_value"
              :placeholder="isMailclass ? 'bulk' : 'gmail.com'"
            />
          </div>
        </div>
        <div v-if="isMailclass" class="space-y-1.5">
          <Label for="rr-match-header">Header Name</Label>
          <Input id="rr-match-header" v-model="form.match_header" placeholder="X-Mail-Class" />
          <p class="text-xs text-muted-foreground">
            Routes mail whose <span class="font-mono">{{ form.match_header || 'X-Mail-Class' }}</span>
            header equals the value above.
          </p>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="rr-target-type">Target Type</Label>
            <Select id="rr-target-type" v-model="form.target_type" @change="onTargetTypeChange">
              <option v-for="t in targetTypes" :key="t.value" :value="t.value">{{ t.label }}</option>
            </Select>
          </div>
          <div class="space-y-1.5">
            <Label for="rr-target-id">Target {{ form.target_type === 'vmta' ? 'VMTA' : 'Group' }}</Label>
            <Select id="rr-target-id" v-model="form.target_id" data-testid="rr-target-id">
              <option value="" disabled>
                {{ targetOptions.length ? 'Select a target…' : 'No targets available' }}
              </option>
              <option v-for="t in targetOptions" :key="t.id" :value="t.id">{{ t.label }}</option>
            </Select>
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="rr-priority">Priority (higher wins)</Label>
            <Input id="rr-priority" v-model.number="form.priority" type="number" />
          </div>
          <div v-if="isEdit" class="space-y-1.5">
            <Label for="rr-status">Status</Label>
            <Select id="rr-status" v-model="form.status">
              <option v-for="s in RULE_STATUSES" :key="s" :value="s">{{ s }}</option>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button
            type="submit"
            :disabled="saving || !form.name || !form.match_value || !form.target_id"
          >
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Create' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
