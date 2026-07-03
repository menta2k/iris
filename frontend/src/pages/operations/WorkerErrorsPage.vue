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
import { usePagedList } from '@/composables/usePagedList'
import { workerErrorsService } from '@/services'
import { formatDateTime } from '@/composables/useTimezone'
import type { WorkerErrorLog, WorkerErrorLogFilters } from '@/types'

const LEVEL_ITEMS = [
  { title: 'All levels', value: '' },
  { title: 'Error', value: 'error' },
  { title: 'Warning', value: 'warn' },
]

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

// Expandable master-detail row (single-expand): stack-trace-like payloads
// need the full width, not a truncated cell.
const expandedId = ref<string | null>(null)

function toggleExpand(id: string) {
  expandedId.value = expandedId.value === id ? null : id
}
</script>

<template>
  <div>
    <PageHeader
      title="Worker Errors"
      description="Warnings and errors emitted by background workers (ingest, DMARC, bounces, ACME). Failures that previously only reached stdout are captured here."
    />

    <Card class="mb-4">
      <CardContent class="pa-4">
        <form @submit.prevent="reload">
          <v-row dense align="end">
            <v-col cols="12" sm="4" md="3">
              <Label for="f-level">Level</Label>
              <v-select
                id="f-level"
                :model-value="filters.level"
                :items="LEVEL_ITEMS"
                data-testid="worker-error-level"
                variant="outlined"
                density="compact"
                hide-details
                @update:model-value="filters.level = $event"
              />
            </v-col>
            <v-col cols="12" sm="4" md="3">
              <Label for="f-worker">Worker</Label>
              <Input id="f-worker" v-model="filters.worker" placeholder="dmarc" />
            </v-col>
            <v-col cols="12" sm="4" md="3" class="d-flex ga-2">
              <Button type="submit" data-testid="apply-filters">Filter</Button>
              <Button type="button" variant="outline" @click="resetFilters">Reset</Button>
            </v-col>
          </v-row>
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
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead style="width: 40px" />
                <TableHead>Time</TableHead>
                <TableHead>Level</TableHead>
                <TableHead>Worker</TableHead>
                <TableHead>Message</TableHead>
                <TableHead>Detail</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <template v-for="w in items" :key="w.id">
                <TableRow class="row-clickable" @click="toggleExpand(w.id)">
                  <TableCell>
                    <v-icon size="small" :icon="expandedId === w.id ? 'mdi-chevron-up' : 'mdi-chevron-down'" />
                  </TableCell>
                  <TableCell class="text-no-wrap text-medium-emphasis">{{ formatDateTime(w.eventTime) }}</TableCell>
                  <TableCell>
                    <StatusBadge :status="w.level === 'warn' ? 'warning' : 'error'" />
                  </TableCell>
                  <TableCell class="font-mono text-caption">{{ w.worker || '—' }}</TableCell>
                  <TableCell style="max-width: 448px">
                    <span class="d-block text-truncate" :title="w.message">{{ w.message }}</span>
                  </TableCell>
                  <TableCell style="max-width: 384px">
                    <code
                      v-if="formatDetail(w.detail)"
                      class="d-block text-truncate font-mono text-caption text-medium-emphasis"
                      :title="formatDetail(w.detail)"
                    >{{ w.detail }}</code>
                    <span v-else class="text-medium-emphasis">—</span>
                  </TableCell>
                </TableRow>
                <tr v-if="expandedId === w.id">
                  <td :colspan="6" class="px-4 py-3">
                    <p class="mb-1 text-caption text-uppercase text-medium-emphasis">Message</p>
                    <code class="d-block pa-2 rounded border font-mono text-caption text-break mb-3">{{
                      w.message
                    }}</code>
                    <template v-if="formatDetail(w.detail)">
                      <p class="mb-1 text-caption text-uppercase text-medium-emphasis">Detail</p>
                      <pre class="detail-pre pa-2 rounded border font-mono text-caption">{{
                        formatDetail(w.detail)
                      }}</pre>
                    </template>
                  </td>
                </tr>
              </template>
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

<style scoped>
.row-clickable {
  cursor: pointer;
}
.detail-pre {
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
