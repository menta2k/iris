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
import type { Bounce } from '@/types'

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
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Recipient</TableHead>
                <TableHead>Mailclass</TableHead>
                <TableHead>SMTP Status</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Diagnostic</TableHead>
                <TableHead>State</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="b in items" :key="b.id">
                <TableCell class="whitespace-nowrap text-muted-foreground">{{ b.eventTime }}</TableCell>
                <TableCell>{{ b.recipient }}</TableCell>
                <TableCell>{{ b.mailclass }}</TableCell>
                <TableCell class="font-mono text-xs">{{ b.smtpStatus }}</TableCell>
                <TableCell><Badge variant="destructive">{{ b.bounceType }}</Badge></TableCell>
                <TableCell class="text-muted-foreground">{{ b.diagnostic }}</TableCell>
                <TableCell><StatusBadge :status="b.processingState" /></TableCell>
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
