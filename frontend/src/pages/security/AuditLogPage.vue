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
import { StatusBadge } from '@/components/ui/badge'
import { usePagedList } from '@/composables/usePagedList'
import { identityAuditService } from '@/services'
import type { AuditEntry } from '@/types'

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
} = usePagedList<AuditEntry>({ loader: (page) => identityAuditService.listAuditEntries(page) })
</script>

<template>
  <div>
    <PageHeader title="Audit Log" description="Immutable record of administrative actions." />
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No audit entries recorded."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Actor</TableHead>
                <TableHead>Operation</TableHead>
                <TableHead>Target</TableHead>
                <TableHead>IP Address</TableHead>
                <TableHead>Outcome</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="a in items" :key="a.id">
                <TableCell class="whitespace-nowrap text-muted-foreground">{{ a.occurredAt }}</TableCell>
                <TableCell class="font-mono text-xs">{{ a.actorUserId }}</TableCell>
                <TableCell class="font-medium">{{ a.operation }}</TableCell>
                <TableCell class="font-mono text-xs">{{ a.targetType }}/{{ a.targetId }}</TableCell>
                <TableCell class="font-mono text-xs">{{ a.ipAddress }}</TableCell>
                <TableCell><StatusBadge :status="a.outcome" /></TableCell>
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
