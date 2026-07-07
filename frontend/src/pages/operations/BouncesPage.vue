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
import { ApiError } from '@/services/http'
import { formatDateTime } from '@/composables/useTimezone'
import type { Bounce, DsnMessage } from '@/types'

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

// Lazy-loaded raw DSN message(s) for dsn-type bounces, keyed by bounce id.
type DsnState = { loading: boolean; error: string | null; messages: DsnMessage[] }
const dsnByBounce = ref<Record<string, DsnState>>({})

async function loadDsn(b: Bounce) {
  if (dsnByBounce.value[b.id]) return // already loaded/loading
  dsnByBounce.value = { ...dsnByBounce.value, [b.id]: { loading: true, error: null, messages: [] } }
  try {
    const res = await mailOperationsService.listDsnMessages(b.recipient)
    dsnByBounce.value = {
      ...dsnByBounce.value,
      [b.id]: { loading: false, error: null, messages: res.items ?? [] },
    }
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to load DSN message.'
    dsnByBounce.value = { ...dsnByBounce.value, [b.id]: { loading: false, error: msg, messages: [] } }
  }
}

function toggleExpand(b: Bounce) {
  expandedId.value = expandedId.value === b.id ? null : b.id
  if (expandedId.value === b.id && b.bounceType === 'dsn') loadDsn(b)
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
                <TableRow class="row-clickable" @click="toggleExpand(b)">
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

                    <template v-if="b.bounceType === 'dsn'">
                      <p class="mt-3 mb-1 text-caption text-uppercase text-medium-emphasis">DSN message</p>
                      <div v-if="dsnByBounce[b.id]?.loading" class="text-caption text-medium-emphasis">Loading…</div>
                      <div v-else-if="dsnByBounce[b.id]?.error" class="text-caption text-error">
                        {{ dsnByBounce[b.id]?.error }}
                      </div>
                      <div
                        v-else-if="!dsnByBounce[b.id]?.messages?.length"
                        class="text-caption text-medium-emphasis"
                      >
                        No archived DSN message for this recipient. Only asynchronous bounces captured after
                        this feature shipped are stored.
                      </div>
                      <div v-for="m in dsnByBounce[b.id]?.messages" v-else :key="m.id" class="mt-1">
                        <div class="text-caption text-medium-emphasis">
                          Received {{ formatDateTime(m.receivedAt) }}
                          <span v-if="m.messageId"> · message {{ m.messageId }}</span>
                        </div>
                        <pre class="dsn-raw">{{ m.rawMessage }}</pre>
                      </div>
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

/* Uniform width so the classification badges line up into a tidy column
   (widths otherwise range from ~104px to ~145px). Higher specificity than the
   Badge component's own min-width so this wins. */
:deep(.v-chip.bounce-classification) {
  min-width: 150px;
}

.dsn-raw {
  max-height: 45vh;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--v-font-monospace, monospace);
  font-size: 0.8125rem;
  line-height: 1.4;
  padding: 0.75rem;
  border-radius: 6px;
  background: rgba(var(--v-theme-on-surface), 0.04);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.12);
}
</style>
