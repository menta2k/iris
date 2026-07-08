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
  { value: 'sender_ip', label: 'Sender IP → assign mail class' },
]
const targetTypes = [
  { value: 'vmta', label: 'VMTA' },
  { value: 'vmta_group', label: 'VMTA Group' },
]
const RULE_STATUSES = ['active', 'disabled']

// v-select item lists ({ title, value }) derived from the enums above.
const matchTypeItems = matchTypes.map((t) => ({ title: t.label, value: t.value }))
const targetTypeItems = targetTypes.map((t) => ({ title: t.label, value: t.value }))
const ruleStatusItems = RULE_STATUSES.map((s) => ({ title: s, value: s }))

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
interface Condition {
  header: string
  value: string
}
function emptyForm() {
  return {
    name: '',
    match_type: 'recipient_domain',
    match_header: 'X-Mail-Class',
    match_value: '',
    conditions: [{ header: 'X-Mail-Class', value: '' }] as Condition[],
    priority: 100,
    target_type: 'vmta_group',
    target_id: '',
    assign_mailclass: '',
    status: 'active',
  }
}
const form = ref(emptyForm())

const isEdit = computed(() => mode.value === 'edit')
const isMailclass = computed(() => form.value.match_type === 'mailclass')
// sender_ip rules classify by IP/CIDR and assign a mailclass; they have no
// VMTA/group target.
const isSenderIP = computed(() => form.value.match_type === 'sender_ip')
// Mailclass rules match on one or more header/value conditions (OR); a rule is
// valid when at least one condition has a value.
const validConditions = computed(() =>
  form.value.conditions.filter((c) => c.value.trim() !== ''),
)
function addCondition() {
  form.value.conditions.push({ header: form.value.conditions[0]?.header || 'X-Mail-Class', value: '' })
}
function removeCondition(i: number) {
  form.value.conditions.splice(i, 1)
  if (form.value.conditions.length === 0) addCondition()
}
// The form is submittable when the type-specific required fields are present.
const canSubmit = computed(() => {
  if (!form.value.name) return false
  if (isMailclass.value) return validConditions.value.length > 0 && !!form.value.target_id
  if (!form.value.match_value) return false
  return isSenderIP.value ? !!form.value.assign_mailclass : !!form.value.target_id
})

// Options for the Target dropdown follow the selected target type.
const targetOptions = computed(() =>
  form.value.target_type === 'vmta'
    ? availableVmtas.value.map((v) => ({ id: v.id, label: `${v.name} (${v.ipAddress})` }))
    : availableGroups.value.map((g) => ({ id: g.id, label: g.name })),
)

// Target dropdown items: a disabled placeholder row plus the options for the
// selected target type.
const targetIdItems = computed(() => [
  {
    title: targetOptions.value.length ? 'Select a target…' : 'No targets available',
    value: '',
    props: { disabled: true },
  },
  ...targetOptions.value.map((t) => ({ title: t.label, value: t.id })),
])

// When the target type changes, drop a now-invalid selection and pick the first
// valid option of the new type.
function onTargetTypeChange() {
  if (!targetOptions.value.some((o) => o.id === form.value.target_id)) {
    form.value.target_id = ''
  }
  ensureTargetSelected()
}

// When switching to a targeted match type, make sure a target is preselected so
// the form is immediately valid.
function onMatchTypeChange() {
  if (form.value.match_type !== 'sender_ip') ensureTargetSelected()
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
  form.value = emptyForm()
  dialogOpen.value = true
  await loadTargets()
}

async function openEdit(r: RoutingRule) {
  mode.value = 'edit'
  editId.value = r.id
  const conditions: Condition[] =
    r.conditions && r.conditions.length
      ? r.conditions.map((c) => ({ header: c.header || 'X-Mail-Class', value: c.value }))
      : [{ header: r.matchHeader || 'X-Mail-Class', value: r.matchValue }]
  form.value = {
    name: r.name,
    match_type: r.matchType,
    match_header: r.matchHeader || 'X-Mail-Class',
    match_value: r.matchValue,
    conditions,
    priority: r.priority,
    target_type: r.targetType || 'vmta_group',
    target_id: r.targetId,
    assign_mailclass: r.assignMailclass || '',
    status: (r.status || 'active').toLowerCase(),
  }
  dialogOpen.value = true
  await loadTargets()
}

async function submit() {
  if (!canSubmit.value) return
  saving.value = true
  try {
    // Mailclass rules send the OR-list of conditions (trimmed, header defaulted);
    // match_header/match_value mirror the first for compatibility. sender_ip
    // rules carry an assigned class and no VMTA/group target.
    const mailclass = form.value.match_type === 'mailclass'
    const senderIP = form.value.match_type === 'sender_ip'
    const conditions = mailclass
      ? validConditions.value.map((c) => ({ header: c.header.trim() || 'X-Mail-Class', value: c.value.trim() }))
      : undefined
    const payload = {
      name: form.value.name,
      match_type: form.value.match_type,
      match_header: mailclass ? conditions![0].header : '',
      match_value: mailclass ? conditions![0].value : form.value.match_value,
      conditions,
      priority: Number(form.value.priority),
      target_type: senderIP ? '' : form.value.target_type,
      target_id: senderIP ? '' : form.value.target_id,
      assign_mailclass: senderIP ? form.value.assign_mailclass : '',
    }
    if (isEdit.value && editId.value) {
      await outboundConfigService.updateRoutingRule(editId.value, {
        ...payload,
        status: form.value.status,
      })
      toast({ title: 'Routing rule updated', description: form.value.name, variant: 'success' })
    } else {
      await outboundConfigService.createRoutingRule(payload)
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
        <CardContent class="pa-0">
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
                <TableCell class="font-weight-medium">{{ r.name }}</TableCell>
                <TableCell>
                  <Badge variant="outline">{{ r.matchType }}</Badge>
                  <span class="ml-2 font-mono text-caption">
                    <template v-if="r.matchType === 'mailclass' && r.conditions && r.conditions.length">
                      <span v-for="(c, i) in r.conditions" :key="i">
                        <template v-if="i > 0"> <span class="text-medium-emphasis">or</span> </template>{{ c.header }}: {{ c.value }}
                      </span>
                    </template>
                    <template v-else>
                      <template v-if="r.matchType === 'mailclass'">{{ r.matchHeader }}: </template>{{ r.matchValue }}
                    </template>
                  </span>
                </TableCell>
                <TableCell>
                  <template v-if="r.matchType === 'sender_ip'">
                    <Badge variant="secondary">mail class</Badge>
                    <span class="ml-2 font-mono text-caption">{{ r.assignMailclass }}</span>
                  </template>
                  <template v-else>
                    <Badge variant="secondary">{{ r.targetType }}</Badge>
                    <span class="ml-2 text-caption">{{ targetName(r) }}</span>
                  </template>
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
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="rr-name">Name</Label>
          <Input id="rr-name" v-model="form.name" placeholder="route-gmail" />
        </div>
        <v-row dense>
          <v-col :cols="isMailclass ? 12 : 6" class="d-flex flex-column ga-1">
            <Label for="rr-match-type">Match Type</Label>
            <v-select
              id="rr-match-type"
              v-model="form.match_type"
              :items="matchTypeItems"
              variant="outlined"
              density="compact"
              hide-details
              @update:model-value="onMatchTypeChange"
            />
          </v-col>
          <v-col v-if="!isMailclass" cols="6" class="d-flex flex-column ga-1">
            <Label for="rr-match-value">{{ isSenderIP ? 'Sender IP / CIDR' : 'Match Value' }}</Label>
            <Input
              id="rr-match-value"
              v-model="form.match_value"
              :placeholder="isSenderIP ? '10.1.111.0/24' : 'gmail.com'"
            />
          </v-col>
        </v-row>
        <!-- Mailclass: one or more header/value conditions, matched with OR. -->
        <div v-if="isMailclass" class="d-flex flex-column ga-2">
          <Label>Match conditions <span class="text-medium-emphasis">(any match — OR)</span></Label>
          <div
            v-for="(c, i) in form.conditions"
            :key="i"
            class="d-flex align-center ga-2"
          >
            <Input
              v-model="c.header"
              class="font-mono"
              style="flex: 1"
              placeholder="X-Mail-Class"
              :data-testid="`rr-cond-header-${i}`"
            />
            <span class="text-medium-emphasis">=</span>
            <Input
              v-model="c.value"
              style="flex: 1"
              placeholder="bulk"
              :data-testid="`rr-cond-value-${i}`"
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              :disabled="form.conditions.length === 1"
              :data-testid="`rr-cond-remove-${i}`"
              @click="removeCondition(i)"
            >
              Remove
            </Button>
          </div>
          <div>
            <Button type="button" variant="outline" size="sm" data-testid="rr-cond-add" @click="addCondition">
              + Add condition
            </Button>
          </div>
          <p class="text-caption text-medium-emphasis">
            The rule routes mail matching <em>any</em> of these header = value pairs.
          </p>
        </div>
        <div v-if="isSenderIP" class="d-flex flex-column ga-1">
          <Label for="rr-assign-mailclass">Assign Mail Class</Label>
          <Input
            id="rr-assign-mailclass"
            v-model="form.assign_mailclass"
            data-testid="rr-assign-mailclass"
            placeholder="test-class"
          />
          <p class="text-caption text-medium-emphasis">
            Mail from this IP/CIDR with no mail-class header is tagged
            <span class="font-mono">{{ form.assign_mailclass || 'test-class' }}</span> and then
            follows that class's routing rule.
          </p>
        </div>
        <v-row v-if="!isSenderIP" dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="rr-target-type">Target Type</Label>
            <v-select
              id="rr-target-type"
              v-model="form.target_type"
              :items="targetTypeItems"
              variant="outlined"
              density="compact"
              hide-details
              @update:model-value="onTargetTypeChange"
            />
          </v-col>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="rr-target-id">Target {{ form.target_type === 'vmta' ? 'VMTA' : 'Group' }}</Label>
            <v-select
              id="rr-target-id"
              v-model="form.target_id"
              :items="targetIdItems"
              data-testid="rr-target-id"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6" class="d-flex flex-column ga-1">
            <Label for="rr-priority">Priority (higher wins)</Label>
            <Input id="rr-priority" v-model.number="form.priority" type="number" />
          </v-col>
          <v-col v-if="isEdit" cols="6" class="d-flex flex-column ga-1">
            <Label for="rr-status">Status</Label>
            <v-select
              id="rr-status"
              v-model="form.status"
              :items="ruleStatusItems"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
        </v-row>
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
