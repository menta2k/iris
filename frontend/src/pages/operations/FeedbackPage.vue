<script setup lang="ts">
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
import { Badge, StatusBadge } from '@/components/ui/badge'
import { usePagedList } from '@/composables/usePagedList'
import { mailOperationsService } from '@/services'
import type { FeedbackReport } from '@/types'

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
} = usePagedList<FeedbackReport>({
  loader: (page) => mailOperationsService.listFeedbackReports(page),
})
</script>

<template>
  <div>
    <PageHeader title="Feedback Reports" description="FBL/ARF complaint reports from mailbox providers." />
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No feedback reports recorded."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Received</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Recipient</TableHead>
                <TableHead>State</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="f in items" :key="f.id">
                <TableCell class="whitespace-nowrap text-muted-foreground">{{ f.receivedAt }}</TableCell>
                <TableCell class="font-mono text-xs">{{ f.source }}</TableCell>
                <TableCell><Badge variant="warning">{{ f.reportType }}</Badge></TableCell>
                <TableCell>{{ f.recipient }}</TableCell>
                <TableCell><StatusBadge :status="f.processingState" /></TableCell>
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
