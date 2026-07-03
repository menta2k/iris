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
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { domainSafetyService } from '@/services'
import { ApiError } from '@/services/http'
import type { Suppression } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<Suppression>({
  loader: () => domainSafetyService.listSuppressions(),
})
const { toast } = useToast()

const SUPPRESSION_STATUSES = ['active', 'disabled', 'expired']
const SUPPRESSION_STATUS_ITEMS = SUPPRESSION_STATUSES.map((s) => ({ title: s, value: s }))
const SUPPRESSION_TYPE_ITEMS = [
  { title: 'email', value: 'email' },
  { title: 'domain', value: 'domain' },
]

const dialogOpen = ref(false)
const saving = ref(false)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)
const form = ref<{
  type: 'email' | 'domain'
  value: string
  reason: string
  status: string
}>({
  type: 'email',
  value: '',
  reason: '',
  status: 'active',
})

const isEdit = computed(() => mode.value === 'edit')

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = { type: 'email', value: '', reason: '', status: 'active' }
  dialogOpen.value = true
}

function openEdit(s: Suppression) {
  mode.value = 'edit'
  editId.value = s.id
  form.value = {
    type: (s.type as 'email' | 'domain') || 'email',
    value: s.value,
    reason: s.reason,
    status: (s.status || 'active').toLowerCase(),
  }
  dialogOpen.value = true
}

async function submit() {
  if (!isEdit.value && !form.value.value) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await domainSafetyService.updateSuppression(editId.value, {
        reason: form.value.reason,
        status: form.value.status,
      })
      toast({ title: 'Suppression updated', description: form.value.value, variant: 'success' })
    } else {
      await domainSafetyService.createSuppression({
        type: form.value.type,
        value: form.value.value,
        reason: form.value.reason,
      })
      toast({ title: 'Suppression added', description: form.value.value, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save suppression.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="Suppressions" description="Recipients and domains suppressed from future delivery.">
      <template #actions>
        <Button data-testid="create-suppression" @click="openCreate">Add Suppression</Button>
      </template>
    </PageHeader>
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No suppressions on record."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Type</TableHead>
                <TableHead>Value</TableHead>
                <TableHead>Reason</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="s in items" :key="s.id">
                <TableCell><Badge variant="outline">{{ s.type }}</Badge></TableCell>
                <TableCell class="font-weight-medium">{{ s.value }}</TableCell>
                <TableCell><Badge variant="destructive">{{ s.reason }}</Badge></TableCell>
                <TableCell class="text-medium-emphasis">{{ s.source }}</TableCell>
                <TableCell><StatusBadge :status="s.status" /></TableCell>
                <TableCell class="text-right">
                  <Button
                    variant="outline"
                    size="sm"
                    :data-testid="`edit-suppression-${s.id}`"
                    @click="openEdit(s)"
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
        <DialogTitle>{{ isEdit ? 'Edit Suppression' : 'Add Suppression' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="supp-type">Type</Label>
          <v-select
            id="supp-type"
            v-model="form.type"
            :items="SUPPRESSION_TYPE_ITEMS"
            :disabled="isEdit"
            variant="outlined"
            density="compact"
            hide-details
          />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="supp-value">Value</Label>
          <Input
            id="supp-value"
            v-model="form.value"
            :disabled="isEdit"
            :placeholder="form.type === 'domain' ? 'example.com' : 'user@example.com'"
          />
          <p v-if="isEdit" class="text-caption text-medium-emphasis">Type and value are immutable.</p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="supp-reason">Reason</Label>
          <Input id="supp-reason" v-model="form.reason" placeholder="hard_bounce" />
        </div>
        <div v-if="isEdit" class="d-flex flex-column ga-1">
          <Label for="supp-status">Status</Label>
          <v-select
            id="supp-status"
            v-model="form.status"
            :items="SUPPRESSION_STATUS_ITEMS"
            variant="outlined"
            density="compact"
            hide-details
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || (!isEdit && !form.value)">
            {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Add Suppression' }}
          </Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
