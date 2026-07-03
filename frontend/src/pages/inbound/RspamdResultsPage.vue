<script setup lang="ts">
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
import { useAsyncList } from '@/composables/useAsyncList'
import { inboundAutomationService } from '@/services'
import { formatDateTime } from '@/composables/useTimezone'
import type { RspamdResult } from '@/types'

const { items, loading, error, notImplemented } = useAsyncList<RspamdResult>({
  loader: () => inboundAutomationService.listRspamdResults(),
})

// Colour-code the rspamd verdict: reject / soft reject → red, header-tagging
// actions → amber, greylist → neutral, no action (clean) → green.
function actionVariant(action: string) {
  switch ((action || '').toLowerCase()) {
    case 'reject':
    case 'soft reject':
      return 'destructive' as const
    case 'add header':
    case 'rewrite subject':
      return 'warning' as const
    case 'no action':
      return 'success' as const
    default:
      return 'secondary' as const
  }
}
</script>

<template>
  <div>
    <PageHeader title="Rspamd Results" description="Spam-scanning verdicts for scanned messages." />
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No Rspamd results recorded."
    >
      <Card>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Recipient</TableHead>
                <TableHead>Message ID</TableHead>
                <TableHead>Score</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Symbols</TableHead>
                <TableHead>Reason</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="r in items" :key="r.id">
                <TableCell class="text-no-wrap text-medium-emphasis">{{ formatDateTime(r.eventTime) }}</TableCell>
                <TableCell class="font-mono text-caption">{{ r.recipient || '—' }}</TableCell>
                <TableCell class="font-mono text-caption text-medium-emphasis">{{ r.messageId || '—' }}</TableCell>
                <TableCell class="tabular-nums">{{ r.score }}</TableCell>
                <TableCell>
                  <Badge :variant="actionVariant(r.action)">{{ r.action }}</Badge>
                </TableCell>
                <TableCell class="font-mono text-caption text-medium-emphasis">{{ r.symbols?.length ? r.symbols.join(', ') : '—' }}</TableCell>
                <TableCell class="text-medium-emphasis">{{ r.reason }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>
  </div>
</template>
