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

function scoreVariant(score: number) {
  if (score >= 15) return 'destructive' as const
  if (score >= 5) return 'warning' as const
  return 'success' as const
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
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Mail Record</TableHead>
                <TableHead>Score</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Symbols</TableHead>
                <TableHead>Reason</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="r in items" :key="r.id">
                <TableCell class="whitespace-nowrap text-muted-foreground">{{ formatDateTime(r.eventTime) }}</TableCell>
                <TableCell class="font-mono text-xs">{{ r.mailRecordId }}</TableCell>
                <TableCell>
                  <Badge :variant="scoreVariant(r.score)">{{ r.score }}</Badge>
                </TableCell>
                <TableCell>{{ r.action }}</TableCell>
                <TableCell class="font-mono text-xs text-muted-foreground">{{ r.symbols?.length ? r.symbols.join(', ') : '—' }}</TableCell>
                <TableCell class="text-muted-foreground">{{ r.reason }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>
  </div>
</template>
