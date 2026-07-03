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
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { feedbackLoopsService } from '@/services'
import { ApiError } from '@/services/http'
import type { FeedbackLoop, FeedbackLoopStatus } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<FeedbackLoop>({
  loader: () => feedbackLoopsService.listFeedbackLoops(),
})
const { toast } = useToast()

const FBL_STATUSES: FeedbackLoopStatus[] = ['awaiting_approval', 'approved']
const FBL_STATUS_ITEMS = FBL_STATUSES.map((s) => ({ title: s, value: s }))

const dialogOpen = ref(false)
const saving = ref(false)
const deletingId = ref<string | null>(null)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)

interface FormState {
  domain: string
  feedback_address: string
  forward_address: string
  status: FeedbackLoopStatus
}

function emptyForm(): FormState {
  return { domain: '', feedback_address: '', forward_address: '', status: 'awaiting_approval' }
}

const form = ref<FormState>(emptyForm())

const isEdit = computed(() => mode.value === 'edit')
const isAwaiting = computed(() => form.value.status === 'awaiting_approval')
const canSubmit = computed(
  () =>
    !!form.value.domain &&
    !!form.value.feedback_address &&
    (!isAwaiting.value || !!form.value.forward_address),
)

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = emptyForm()
  dialogOpen.value = true
}

function openEdit(f: FeedbackLoop) {
  mode.value = 'edit'
  editId.value = f.id
  form.value = {
    domain: f.domain,
    feedback_address: f.feedbackAddress,
    forward_address: f.forwardAddress,
    status: (f.status as FeedbackLoopStatus) || 'awaiting_approval',
  }
  dialogOpen.value = true
}

async function submit() {
  if (!canSubmit.value) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await feedbackLoopsService.updateFeedbackLoop(editId.value, { ...form.value })
      toast({ title: 'Feedback loop updated', description: form.value.feedback_address, variant: 'success' })
    } else {
      await feedbackLoopsService.createFeedbackLoop({ ...form.value })
      toast({ title: 'Feedback loop created', description: form.value.feedback_address, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save feedback loop.'
    toast({ title: isEdit.value ? 'Update failed' : 'Create failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

async function remove(f: FeedbackLoop) {
  deletingId.value = f.id
  try {
    await feedbackLoopsService.deleteFeedbackLoop(f.id)
    toast({ title: 'Feedback loop removed', description: f.feedbackAddress, variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to delete feedback loop.'
    toast({ title: 'Delete failed', description: msg, variant: 'destructive' })
  } finally {
    deletingId.value = null
  }
}
</script>

<template>
  <div>
    <PageHeader
      title="Feedback Loops"
      description="Per-domain FBL enrollments. While awaiting approval, feedback mail is forwarded to a human so the provider's confirmation email can be read; once approved the domain enables the built-in ARF parser."
    >
      <template #actions>
        <Button data-testid="create-feedback-loop" @click="openCreate">New Feedback Loop</Button>
      </template>
    </PageHeader>
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No feedback loops configured."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead>Feedback Address</TableHead>
                <TableHead>Forward Address</TableHead>
                <TableHead>Status</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="f in items" :key="f.id">
                <TableCell class="font-weight-medium">{{ f.domain }}</TableCell>
                <TableCell class="font-mono text-caption">{{ f.feedbackAddress }}</TableCell>
                <TableCell class="font-mono text-caption">{{ f.forwardAddress || '—' }}</TableCell>
                <TableCell><StatusBadge :status="f.status" /></TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-2">
                    <Button
                      variant="outline"
                      size="sm"
                      :data-testid="`edit-feedback-loop-${f.id}`"
                      @click="openEdit(f)"
                    >
                      Edit
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      :disabled="deletingId === f.id"
                      :data-testid="`delete-feedback-loop-${f.id}`"
                      @click="remove(f)"
                    >
                      {{ deletingId === f.id ? 'Removing…' : 'Remove' }}
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
        <DialogTitle>{{ isEdit ? 'Edit Feedback Loop' : 'Create Feedback Loop' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="fbl-domain">Domain</Label>
          <Input id="fbl-domain" v-model="form.domain" placeholder="fbl.example.com" />
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="fbl-feedback">Feedback Address</Label>
          <Input id="fbl-feedback" v-model="form.feedback_address" placeholder="fbl@fbl.example.com" />
          <p class="text-caption text-medium-emphasis">Must be an address at the domain above.</p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="fbl-status">Status</Label>
          <v-select
            id="fbl-status"
            v-model="form.status"
            :items="FBL_STATUS_ITEMS"
            variant="outlined"
            density="compact"
            hide-details
          />
        </div>
        <div v-if="isAwaiting" class="d-flex flex-column ga-1">
          <Label for="fbl-forward">Forward Address</Label>
          <Input id="fbl-forward" v-model="form.forward_address" placeholder="ops@example.com" />
          <p class="text-caption text-medium-emphasis">
            Mail to the feedback address is forwarded here until the loop is approved.
          </p>
        </div>
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
