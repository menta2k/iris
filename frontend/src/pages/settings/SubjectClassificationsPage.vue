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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { classificationsService } from '@/services'
import { ApiError } from '@/services/http'
import type { SubjectClassification } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<SubjectClassification>({
  loader: () => classificationsService.list(),
})
const { toast } = useToast()

// Colour-code the Source column: manual rules (human-authored) → indigo,
// AI-generated labels → green. Unknown sources fall back to neutral.
const SOURCE_VARIANT: Record<string, 'default' | 'success'> = {
  manual: 'default',
  ai: 'success',
}
function sourceVariant(source: string) {
  return SOURCE_VARIANT[(source || '').toLowerCase()] ?? 'secondary'
}
function sourceLabel(source: string) {
  const s = (source || '').toLowerCase()
  if (s === 'ai') return 'AI'
  return s ? s.charAt(0).toUpperCase() + s.slice(1) : 'Unknown'
}

const dialogOpen = ref(false)
const saving = ref(false)
const deletingId = ref<string | null>(null)
const mode = ref<'create' | 'edit'>('create')
const editId = ref<string | null>(null)

interface FormState {
  subject: string
  label: string
}

function emptyForm(): FormState {
  return { subject: '', label: '' }
}

const form = ref<FormState>(emptyForm())
const isEdit = computed(() => mode.value === 'edit')
const canSubmit = computed(() => !!form.value.subject.trim() && !!form.value.label.trim())

function openCreate() {
  mode.value = 'create'
  editId.value = null
  form.value = emptyForm()
  dialogOpen.value = true
}

function openEdit(c: SubjectClassification) {
  mode.value = 'edit'
  editId.value = c.id
  form.value = { subject: c.subject, label: c.label }
  dialogOpen.value = true
}

async function submit() {
  if (!canSubmit.value) return
  saving.value = true
  try {
    if (isEdit.value && editId.value) {
      await classificationsService.update(editId.value, { id: editId.value, ...form.value })
      toast({ title: 'Classification updated', description: form.value.label, variant: 'success' })
    } else {
      await classificationsService.create({ ...form.value })
      toast({ title: 'Classification created', description: form.value.label, variant: 'success' })
    }
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to save classification.'
    toast({
      title: isEdit.value ? 'Update failed' : 'Create failed',
      description: msg,
      variant: 'destructive',
    })
  } finally {
    saving.value = false
  }
}

async function remove(c: SubjectClassification) {
  deletingId.value = c.id
  try {
    await classificationsService.remove(c.id)
    toast({ title: 'Classification removed', description: c.label, variant: 'success' })
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to delete classification.'
    toast({ title: 'Delete failed', description: msg, variant: 'destructive' })
  } finally {
    deletingId.value = null
  }
}
</script>

<template>
  <div>
    <PageHeader
      title="Subject Classifications"
      description="Rules mapping email subjects to a short label. These seed and cache the optional subject-classification feature: an incoming subject is matched against them by trigram similarity, and AI-generated labels are added here automatically. Enable the feature under Global Settings."
    >
      <template #actions>
        <Button data-testid="create-classification" @click="openCreate">New Rule</Button>
      </template>
    </PageHeader>
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No classification rules yet. They appear here as you add rules or the AI labels new subjects."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Subject</TableHead>
                <TableHead>Label</TableHead>
                <TableHead>Source</TableHead>
                <TableHead class="text-right">Hits</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="c in items" :key="c.id">
                <TableCell class="text-truncate" style="max-width: 448px" :title="c.subject">{{ c.subject }}</TableCell>
                <TableCell class="font-weight-medium">{{ c.label }}</TableCell>
                <TableCell><Badge :variant="sourceVariant(c.source)">{{ sourceLabel(c.source) }}</Badge></TableCell>
                <TableCell class="text-right tabular-nums">{{ c.hitCount }}</TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-2">
                    <Button
                      variant="outline"
                      size="sm"
                      :data-testid="`edit-classification-${c.id}`"
                      @click="openEdit(c)"
                    >
                      Edit
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      :disabled="deletingId === c.id"
                      :data-testid="`delete-classification-${c.id}`"
                      @click="remove(c)"
                    >
                      {{ deletingId === c.id ? 'Removing…' : 'Remove' }}
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
        <DialogTitle>{{ isEdit ? 'Edit Classification' : 'Create Classification' }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="submit">
        <div class="d-flex flex-column ga-1">
          <Label for="cls-subject">Subject</Label>
          <Input id="cls-subject" v-model="form.subject" placeholder="Your order has shipped" />
          <p class="text-caption text-medium-emphasis">
            A representative subject. It is normalized (digits/prefixes stripped) into a matching
            key, so similar subjects match this rule.
          </p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="cls-label">Label</Label>
          <Input id="cls-label" v-model="form.label" placeholder="shipping update" />
          <p class="text-caption text-medium-emphasis">One or two words. Longer input is truncated.</p>
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
