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
import { Select } from '@/components/ui/select'
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
  const first = availableVmtas.value[0]
  members.value.push({ vmta_id: first?.id ?? '', weight: 1 })
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
                  <div class="flex flex-wrap gap-1">
                    <Badge v-for="m in g.members" :key="m.vmtaId" variant="secondary">
                      {{ m.vmtaId }} · w{{ m.weight }}
                    </Badge>
                    <span v-if="!g.members?.length" class="text-xs text-muted-foreground">—</span>
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
      <form class="space-y-4" @submit.prevent="submit">
        <div class="space-y-1.5">
          <Label for="group-name">Name</Label>
          <Input id="group-name" v-model="name" placeholder="pool-marketing" />
        </div>

        <div v-if="isEdit" class="space-y-1.5">
          <Label for="group-status">Status</Label>
          <Select id="group-status" v-model="status">
            <option v-for="s in GROUP_STATUSES" :key="s" :value="s">{{ s }}</option>
          </Select>
        </div>

        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <Label>Weighted Members</Label>
            <Button type="button" variant="outline" size="sm" @click="addMember">Add member</Button>
          </div>
          <p v-if="members.length === 0" class="text-xs text-muted-foreground">
            Add at least one VMTA member.
          </p>
          <div v-for="(m, i) in members" :key="i" class="flex items-center gap-2">
            <Select v-model="m.vmta_id" class="flex-1">
              <option v-for="v in availableVmtas" :key="v.id" :value="v.id">
                {{ v.name }} ({{ v.ipAddress }})
              </option>
            </Select>
            <Input v-model.number="m.weight" type="number" class="w-24" placeholder="weight" />
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
