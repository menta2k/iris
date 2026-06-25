<script setup lang="ts">
import { ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import PaginationControls from '@/components/common/PaginationControls.vue'
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
import { Select } from '@/components/ui/select'
import { usePagedList } from '@/composables/usePagedList'
import { workerErrorsService } from '@/services'
import { formatDateTime } from '@/composables/useTimezone'
import type { WorkerErrorLog, WorkerErrorLogFilters } from '@/types'

const filters = ref<WorkerErrorLogFilters>({
  level: '',
  worker: '',
})

// The loader reads filters at call time, so reload() applies the current values.
const {
  items,
  loading,
  error,
  notImplemented,
  pageSize,
  pageNumber,
  hasPrev,
  hasNext,
  reload,
  nextPage,
  prevPage,
  setPageSize,
} = usePagedList<WorkerErrorLog>({
  loader: (page) => workerErrorsService.listWorkerErrorLogs({ ...filters.value }, page),
})

function resetFilters() {
  filters.value = { level: '', worker: '' }
  reload()
}

// Pretty-print the JSON detail string for the row title/expanded view.
function formatDetail(detail: string): string {
  if (!detail || detail === '{}') return ''
  try {
    return JSON.stringify(JSON.parse(detail), null, 2)
  } catch {
    return detail
  }
}
</script>

<template>
  <div>
    <PageHeader
      title="Worker Errors"
      description="Warnings and errors emitted by background workers (ingest, DMARC, bounces, ACME). Failures that previously only reached stdout are captured here."
    />

    <Card class="mb-4">
      <CardContent class="p-4">
        <form class="grid items-end gap-3 md:grid-cols-4" @submit.prevent="reload">
          <div class="space-y-1">
            <Label for="f-level">Level</Label>
            <Select id="f-level" v-model="filters.level" data-testid="worker-error-level">
              <option value="">All levels</option>
              <option value="error">Error</option>
              <option value="warn">Warning</option>
            </Select>
          </div>
          <div class="space-y-1">
            <Label for="f-worker">Worker</Label>
            <Input id="f-worker" v-model="filters.worker" placeholder="dmarc" />
          </div>
          <div class="flex gap-2">
            <Button type="submit" data-testid="apply-filters">Filter</Button>
            <Button type="button" variant="outline" @click="resetFilters">Reset</Button>
          </div>
        </form>
      </CardContent>
    </Card>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No worker errors recorded — everything is processing cleanly."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Level</TableHead>
                <TableHead>Worker</TableHead>
                <TableHead>Message</TableHead>
                <TableHead>Detail</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="w in items" :key="w.id">
                <TableCell class="whitespace-nowrap text-muted-foreground">{{ formatDateTime(w.eventTime) }}</TableCell>
                <TableCell>
                  <StatusBadge :status="w.level === 'warn' ? 'warning' : 'error'" />
                </TableCell>
                <TableCell class="font-mono text-xs">{{ w.worker || '—' }}</TableCell>
                <TableCell class="max-w-md">
                  <span class="block truncate" :title="w.message">{{ w.message }}</span>
                </TableCell>
                <TableCell class="max-w-md">
                  <code
                    v-if="formatDetail(w.detail)"
                    class="block truncate font-mono text-xs text-muted-foreground"
                    :title="formatDetail(w.detail)"
                  >{{ w.detail }}</code>
                  <span v-else class="text-muted-foreground">—</span>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>

    <PaginationControls
      v-if="!notImplemented && (items.length > 0 || hasPrev)"
      :page-number="pageNumber"
      :has-prev="hasPrev"
      :has-next="hasNext"
      :loading="loading"
      :page-size="pageSize"
      @prev="prevPage"
      @next="nextPage"
      @page-size-change="setPageSize"
    />
  </div>
</template>
