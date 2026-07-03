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
import type { VMTA, VMTAGroup, VMTAGroupMemberInput } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<VMTAGroup>({
  loader: () => outboundConfigService.listVmtaGroups(),
})
const { toast } = useToast()

const GROUP_STATUSES = ['active', 'disabled']
const groupStatusItems = GROUP_STATUSES.map((s) => ({ title: s, value: s }))

const availableVmtas = ref<VMTA[]>([])
const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const name = ref('')
const status = ref('active')
const members = ref<VMTAGroupMemberInput[]>([])

const isEdit = computed(() => mode.value === 'edit')

async function loadVmtas() {
  try {
    const res = await outboundConfigService.listVmtas()
    availableVmtas.value = res.items ?? []
  } catch {
    availableVmtas.value = []
  }
}

// VMTAs are loaded on mount too, so the table can resolve member ids to names.
onMounted(loadVmtas)

// Resolve a member's VMTA id to a readable "name (ip)"; falls back to the id.
function vmtaLabel(id: string): string {
  const v = availableVmtas.value.find((x) => x.id === id)
  return v ? `${v.name} (${v.ipAddress})` : id
}

// A member's weight as a percentage of the pool's total weight.
function weightPct(weight: number, list: ReadonlyArray<{ weight: number }>): number {
  const total = list.reduce((s, m) => s + (Number(m.weight) || 0), 0)
  return total > 0 ? Math.round((Number(weight) / total) * 100) : 0
}

// VMTAs already chosen by a member row, so other rows can't pick a duplicate
// (the backend rejects VMTA_GROUP_MEMBER_DUPLICATE).
const chosenVmtaIds = computed(() => new Set(members.value.map((m) => m.vmta_id).filter(Boolean)))

// v-select items for one member row: VMTAs already picked by ANOTHER row are
// disabled so the operator can't create a duplicate member.
function memberVmtaItems(currentId: string) {
  return availableVmtas.value.map((v) => ({
    title: `${v.name} (${v.ipAddress})`,
    value: v.id,
    props: { disabled: currentId !== v.id && chosenVmtaIds.value.has(v.id) },
  }))
}

async function openCreate() {
  mode.value = 'create'
  editId.value = null
  name.value = ''
  status.value = 'active'
  members.value = []
  dialogOpen.value = true
  await loadVmtas()
}

async function openEdit(g: VMTAGroup) {
  mode.value = 'edit'
  editId.value = g.id
  name.value = g.name
  status.value = (g.status || 'active').toLowerCase()
  members.value = (g.members ?? []).map((m) => ({ vmta_id: m.vmtaId, weight: m.weight }))
  dialogOpen.value = true
  await loadVmtas()
}

function addMember() {
  // Default to the first VMTA not already in the group to avoid an instant dupe.
  const taken = chosenVmtaIds.value
  const next = availableVmtas.value.find((v) => !taken.has(v.id)) ?? availableVmtas.value[0]
  members.value.push({ vmta_id: next?.id ?? '', weight: 1 })
}

function removeMember(index: number) {
  members.value.splice(index, 1)
}

async function submit() {
  if (!name.value || members.value.length === 0) return
  saving.value = true
  const payloadMembers = members.value.map((m) => ({
    vmta_id: m.vmta_id,
    weight: Number(m.weight),
  }))
  try {
    if (isEdit.value && editId.value) {
      await outboundConfigService.updateVmtaGroup(editId.value, {
        name: name.value,
        status: status.value,
        members: payloadMembers,
      })
      toast({ title: 'VMTA group updated', description: name.value, variant: 'success' })
    } else {
      await outboundConfigService.createVmtaGroup({
        name: name.value,
        members: payloadMembers,
      })
      toast({ title: 'VMTA group created', description: name.value, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save group.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="VMTA Groups" description="Weighted pools of VMTAs for load distribution.">
      <template #actions>
        <Button data-testid="create-vmta-group" @click="openCreate">New Group</Button>
      </template>
    </PageHeader>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No VMTA groups configured yet."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Members</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="g in items" :key="g.id">
                <TableCell class="font-medium">{{ g.name }}</TableCell>
                <TableCell>
                  <div class="d-flex flex-wrap ga-1">
                    <Badge v-for="m in g.members" :key="m.vmtaId" variant="secondary">
                      {{ vmtaLabel(m.vmtaId) }} · {{ weightPct(m.weight, g.members) }}%
                    </Badge>
                    <span v-if="!g.members?.length" class="text-caption text-medium-emphasis">—</span>
                  </div>
                </TableCell>
                <TableCell><StatusBadge :status="g.status" /></TableCell>
                <TableCell class="text-right">
                  <Button
                    variant="outline"
                    size="sm"
                    :data-testid="`edit-vmta-group-${g.id}`"
                    @click="openEdit(g)"
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
        <DialogTitle>{{ isEdit ? 'Edit VMTA Group' : 'Create VMTA Group' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="group-name">Name</Label>
          <Input id="group-name" v-model="name" placeholder="pool-marketing" />
        </div>

        <div v-if="isEdit" class="d-flex flex-column ga-1">
          <Label for="group-status">Status</Label>
          <v-select
            id="group-status"
            v-model="status"
            :items="groupStatusItems"
            variant="outlined"
            density="compact"
            hide-details
          />
        </div>

        <div class="d-flex flex-column ga-2">
          <div class="d-flex align-center justify-space-between">
            <Label>Weighted Members</Label>
            <Button type="button" variant="outline" size="sm" @click="addMember">Add member</Button>
          </div>
          <p v-if="availableVmtas.length === 0" class="text-caption text-medium-emphasis">
            No VMTAs available — create a VMTA first.
          </p>
          <p v-else-if="members.length === 0" class="text-caption text-medium-emphasis">
            Add at least one VMTA member. Weights are relative; each member's share of the pool is
            shown as a percentage.
          </p>
          <div v-for="(m, i) in members" :key="i" class="d-flex align-center ga-2">
            <v-select
              v-model="m.vmta_id"
              :items="memberVmtaItems(m.vmta_id)"
              variant="outlined"
              density="compact"
              hide-details
              class="flex-grow-1"
            />
            <Input v-model.number="m.weight" type="number" min="1" class="w-24" placeholder="weight" />
            <span class="text-right text-caption tabular-nums text-medium-emphasis" style="width: 40px">
              {{ weightPct(m.weight, members) }}%
            </span>
            <Button type="button" variant="ghost" size="icon" @click="removeMember(i)">×</Button>
          </div>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !name || members.length === 0">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Create' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
