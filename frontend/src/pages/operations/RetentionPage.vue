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
import { formatDateTime } from '@/composables/useTimezone'
import { retentionService } from '@/services'
import { ApiError } from '@/services/http'
import type { RetentionView } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<RetentionView>({
  loader: () => retentionService.listRetention(),
})
const { toast } = useToast()

function formatBytes(n: number): string {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), units.length - 1)
  return `${(n / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function keepLabel(v: RetentionView): string {
  return v.policy.retentionDays > 0 ? `${v.policy.retentionDays}d` : 'forever'
}
function compressLabel(v: RetentionView): string {
  return v.policy.compressAfterDays > 0 ? `${v.policy.compressAfterDays}d` : '—'
}

const dialogOpen = ref(false)
const saving = ref(false)
const editTable = ref('')
const form = ref({ retention_days: 0, compress_after_days: 0, enabled: true })

function openEdit(v: RetentionView) {
  editTable.value = v.policy.tableName
  form.value = {
    retention_days: v.policy.retentionDays,
    compress_after_days: v.policy.compressAfterDays,
    enabled: v.policy.enabled,
  }
  dialogOpen.value = true
}

const formError = computed(() => {
  const keep = Number(form.value.retention_days)
  const comp = Number(form.value.compress_after_days)
  if (keep < 0 || comp < 0) return 'Values must be 0 or greater.'
  if (keep > 0 && comp > 0 && comp >= keep)
    return 'Compress-after must be less than the retention window.'
  return ''
})

async function save() {
  if (formError.value) return
  saving.value = true
  try {
    await retentionService.updateRetention(editTable.value, {
      table_name: editTable.value,
      retention_days: Number(form.value.retention_days) || 0,
      compress_after_days: Number(form.value.compress_after_days) || 0,
      enabled: form.value.enabled,
    })
    toast({ title: 'Retention updated', description: editTable.value, variant: 'success' })
    dialogOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to update retention.'
    toast({ title: 'Update failed', description: msg, variant: 'destructive' })
  } finally {
    saving.value = false
  }
}

async function run(tableName: string) {
  const label = tableName || 'all tables'
  if (!window.confirm(`Run cleanup now for ${label}? Old chunks will be compressed and dropped.`))
    return
  try {
    await retentionService.runRetention(tableName)
    toast({
      title: 'Cleanup started',
      description: `${label} — refresh in a moment to see the result.`,
      variant: 'success',
    })
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to start cleanup.'
    toast({ title: 'Run failed', description: msg, variant: 'destructive' })
  }
}
</script>

<template>
  <div>
    <PageHeader
      title="Retention"
      description="Per-table cleanup of the event logs. Old TimescaleDB chunks are compressed, then dropped — disk is returned to the OS immediately."
    >
      <template #actions>
        <Button variant="outline" data-testid="run-all-retention" @click="run('')">Run all now</Button>
      </template>
    </PageHeader>
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No retention-managed tables."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Table</TableHead>
                <TableHead>On disk</TableHead>
                <TableHead>Chunks</TableHead>
                <TableHead>Oldest data</TableHead>
                <TableHead>Keep</TableHead>
                <TableHead>Compress&nbsp;after</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Last run</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="v in items" :key="v.policy.tableName">
                <TableCell>
                  <div class="font-weight-medium">{{ v.label }}</div>
                  <div class="font-mono text-caption text-medium-emphasis">{{ v.policy.tableName }}</div>
                </TableCell>
                <template v-if="v.hypertable">
                  <TableCell class="tabular-nums">
                    {{ formatBytes(v.totalBytes) }}
                    <span v-if="v.compressedBytes > 0" class="text-caption text-medium-emphasis">
                      ({{ formatBytes(v.compressedBytes) }} compressed)
                    </span>
                  </TableCell>
                  <TableCell class="tabular-nums">
                    {{ v.chunkCount }}
                    <span v-if="v.compressedChunks > 0" class="text-caption text-medium-emphasis">
                      ({{ v.compressedChunks }} comp.)
                    </span>
                  </TableCell>
                  <TableCell class="text-no-wrap text-medium-emphasis">
                    {{ v.oldestData ? formatDateTime(v.oldestData) : '—' }}
                  </TableCell>
                  <TableCell>
                    <Badge :variant="v.policy.retentionDays > 0 ? 'secondary' : 'outline'">{{ keepLabel(v) }}</Badge>
                  </TableCell>
                  <TableCell class="tabular-nums">{{ compressLabel(v) }}</TableCell>
                  <TableCell>
                    <Badge :variant="v.policy.enabled ? 'success' : 'outline'">
                      {{ v.policy.enabled ? 'enabled' : 'disabled' }}
                    </Badge>
                  </TableCell>
                  <TableCell class="text-no-wrap text-caption text-medium-emphasis">
                    <template v-if="v.lastRun">
                      <span v-if="v.lastRun.error" class="text-error">error</span>
                      <span v-else>
                        {{ formatDateTime(v.lastRun.startedAt) }} ·
                        −{{ formatBytes(Math.max(0, v.lastRun.bytesBefore - v.lastRun.bytesAfter)) }}
                      </span>
                    </template>
                    <template v-else>—</template>
                  </TableCell>
                  <TableCell class="text-right">
                    <div class="d-flex justify-end ga-2">
                      <Button variant="outline" size="sm" :data-testid="`edit-retention-${v.policy.tableName}`" @click="openEdit(v)">Edit</Button>
                      <Button variant="outline" size="sm" :data-testid="`run-retention-${v.policy.tableName}`" @click="run(v.policy.tableName)">Run</Button>
                    </div>
                  </TableCell>
                </template>
                <template v-else>
                  <TableCell colspan="8" class="text-medium-emphasis">
                    TimescaleDB not enabled for this table — chunk-based retention unavailable.
                  </TableCell>
                </template>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>

    <Dialog v-model:open="dialogOpen">
      <DialogHeader>
        <DialogTitle>Edit retention — {{ editTable }}</DialogTitle>
      </DialogHeader>
      <form class="d-flex flex-column ga-4" @submit.prevent="save">
        <div class="d-flex flex-column ga-1">
          <Label for="ret-keep">Keep (days)</Label>
          <Input id="ret-keep" v-model="form.retention_days" type="number" min="0" />
          <p class="text-caption text-medium-emphasis">Drop chunks older than this. <strong>0 = keep forever.</strong></p>
        </div>
        <div class="d-flex flex-column ga-1">
          <Label for="ret-compress">Compress after (days)</Label>
          <Input id="ret-compress" v-model="form.compress_after_days" type="number" min="0" />
          <p class="text-caption text-medium-emphasis">
            Compress chunks older than this (~90% smaller) before they are dropped. 0 = no compression. Must be less than the keep window.
          </p>
        </div>
        <label class="d-flex align-center ga-2 text-body-2">
          <input v-model="form.enabled" type="checkbox" style="width: 16px; height: 16px" data-testid="ret-enabled" />
          Enabled (run automatically each day)
        </label>
        <p v-if="formError" class="text-caption text-error">{{ formError }}</p>
        <DialogFooter>
          <Button type="button" variant="outline" @click="dialogOpen = false">Cancel</Button>
          <Button type="submit" :disabled="saving || !!formError">{{ saving ? 'Saving…' : 'Save' }}</Button>
        </DialogFooter>
      </form>
    </Dialog>
  </div>
</template>
