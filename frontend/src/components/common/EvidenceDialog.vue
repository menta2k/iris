<script setup lang="ts">
import { ref, watch } from 'vue'
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { mailOperationsService, type ActionEvidence } from '@/services/mail-operations'
import { ApiError } from '@/services/http'

const props = defineProps<{
  open: boolean
  subjectType: string // 'tls_policy' | 'suppression'
  subjectKey: string // domain | recipient
  title?: string
}>()
const emit = defineEmits<{ 'update:open': [boolean] }>()

const loading = ref(false)
const error = ref<string | null>(null)
const items = ref<ActionEvidence[]>([])

async function load() {
  if (!props.subjectKey) return
  loading.value = true
  error.value = null
  items.value = []
  try {
    const res = await mailOperationsService.listActionEvidence(props.subjectType, props.subjectKey)
    items.value = res.items ?? []
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : 'Failed to load evidence.'
  } finally {
    loading.value = false
  }
}

// (Re)load whenever the dialog is opened for a subject.
watch(
  () => [props.open, props.subjectKey],
  ([open]) => {
    if (open) load()
  },
)

function fmt(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString(undefined, { hour12: false })
}

// Parse the stored event JSON into ordered key/value rows for display.
function eventRows(ev: ActionEvidence): Array<[string, string]> {
  try {
    const obj = JSON.parse(ev.eventJson || '{}') as Record<string, unknown>
    return Object.entries(obj)
      .filter(([, v]) => v !== '' && v != null)
      .map(([k, v]) => [k, String(v)])
  } catch {
    return []
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogHeader>
      <DialogTitle>{{ title ?? 'Action evidence' }}</DialogTitle>
    </DialogHeader>
    <div class="pa-4" style="max-height: 60vh; overflow-y: auto">
      <p class="text-caption text-medium-emphasis mb-3">
        The exact mail-log event(s) that triggered the automatic action for
        <strong>{{ subjectKey }}</strong>.
      </p>
      <div v-if="loading" class="text-medium-emphasis">Loading…</div>
      <div v-else-if="error" class="text-error">{{ error }}</div>
      <div v-else-if="items.length === 0" class="text-medium-emphasis">
        No evidence recorded for this item. (Actions taken before evidence tracking, or added
        manually, have none.)
      </div>
      <div v-else class="d-flex flex-column ga-4">
        <div v-for="ev in items" :key="ev.id" class="border rounded pa-3">
          <div class="d-flex align-center justify-space-between mb-2">
            <Badge variant="secondary">{{ ev.actionType }}</Badge>
            <span class="text-caption text-medium-emphasis">{{ fmt(ev.createdAt) }}</span>
          </div>
          <p v-if="ev.reason" class="text-body-2 mb-2">{{ ev.reason }}</p>
          <table class="text-caption" style="width: 100%">
            <tbody>
              <tr v-for="[k, v] in eventRows(ev)" :key="k">
                <td class="pr-3 text-medium-emphasis text-no-wrap" style="vertical-align: top">{{ k }}</td>
                <td class="font-mono" style="word-break: break-word">{{ v }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
    <DialogFooter>
      <Button type="button" variant="outline" @click="emit('update:open', false)">Close</Button>
    </DialogFooter>
  </Dialog>
</template>
