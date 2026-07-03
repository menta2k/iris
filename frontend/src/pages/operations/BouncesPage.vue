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
import { Badge } from '@/components/ui/badge'
import { usePagedList } from '@/composables/usePagedList'
import { mailOperationsService } from '@/services'
import { formatDateTime } from '@/composables/useTimezone'
import type { Bounce } from '@/types'

// Colour-code the processing State so each state reads as a distinct colour:
// processed → green, new → indigo (unhandled/info), processing/pending → amber,
// suppressed → red, retried → neutral grey.
function stateVariant(state: string) {
  switch ((state || '').toLowerCase()) {
    case 'processed':
      return 'success' as const
    case 'new':
      return 'default' as const
    case 'processing':
    case 'pending':
      return 'warning' as const
    case 'suppressed':
      return 'destructive' as const
    case 'retried':
      return 'secondary' as const
    default:
      return 'secondary' as const
  }
}
function stateLabel(state: string) {
  const s = (state || '').toString()
  return s ? s.charAt(0).toUpperCase() + s.slice(1).toLowerCase() : '—'
}

const {
  items,
  loading,
  error,
  notImplemented,
  pageSize,
  pageNumber,
  hasPrev,
  hasNext,
  nextPage,
  prevPage,
  setPageSize,
} = usePagedList<Bounce>({ loader: (page) => mailOperationsService.listBounces(page) })

// Expandable master-detail row (single-expand): the full diagnostic is long,
// the row shows a truncated preview and the expanded row the whole payload.
const expandedId = ref<string | null>(null)

function toggleExpand(id: string) {
  expandedId.value = expandedId.value === id ? null : id
}
</script>

<template>
  <div>
    <PageHeader title="Bounces" description="Hard and soft bounces captured from delivery attempts." />
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No bounces recorded."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead style="width: 40px" />
                <TableHead>Time</TableHead>
                <TableHead>Recipient</TableHead>
                <TableHead>Mailclass</TableHead>
                <TableHead>SMTP Status</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Classification</TableHead>
                <TableHead>Diagnostic</TableHead>
                <TableHead>State</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <template v-for="b in items" :key="b.id">
                <TableRow class="row-clickable" @click="toggleExpand(b.id)">
                  <TableCell>
                    <v-icon size="small" :icon="expandedId === b.id ? 'mdi-chevron-up' : 'mdi-chevron-down'" />
                  </TableCell>
                  <TableCell class="text-no-wrap text-medium-emphasis">{{ formatDateTime(b.eventTime) }}</TableCell>
                  <TableCell>{{ b.recipient }}</TableCell>
                  <TableCell>{{ b.mailclass }}</TableCell>
                  <TableCell class="font-mono text-caption">{{ b.smtpStatus }}</TableCell>
                  <TableCell>
                    <Badge v-if="b.bounceType" variant="outline">{{ b.bounceType }}</Badge>
                    <span v-else class="text-medium-emphasis">—</span>
                  </TableCell>
                  <TableCell>
                    <Badge v-if="b.classification" variant="outline" class="bounce-classification">{{ b.classification }}</Badge>
                    <span v-else class="text-medium-emphasis">—</span>
                  </TableCell>
                  <TableCell style="max-width: 384px">
                    <span class="d-block text-truncate text-medium-emphasis" :title="b.diagnostic">{{
                      b.diagnostic
                    }}</span>
                  </TableCell>
                  <TableCell><Badge :variant="stateVariant(b.processingState)">{{ stateLabel(b.processingState) }}</Badge></TableCell>
                </TableRow>
                <tr v-if="expandedId === b.id">
                  <td :colspan="9" class="px-4 py-3">
                    <p class="mb-1 text-caption text-uppercase text-medium-emphasis">Full diagnostic</p>
                    <code class="d-block pa-2 rounded border font-mono text-caption text-break">{{
                      b.diagnostic || '—'
                    }}</code>
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

/* Uniform width so the classification badges line up into a tidy column
   (widths otherwise range from ~104px to ~145px). Higher specificity than the
   Badge component's own min-width so this wins. */
:deep(.v-chip.bounce-classification) {
  min-width: 150px;
}
</style>
