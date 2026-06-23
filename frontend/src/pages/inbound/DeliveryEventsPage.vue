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
import { StatusBadge, Badge } from '@/components/ui/badge'
import { useAsyncList } from '@/composables/useAsyncList'
import { inboundAutomationService } from '@/services'
import { formatDateTime } from '@/composables/useTimezone'
import type { WebhookDeliveryEvent } from '@/types'

const { items, loading, error, notImplemented } = useAsyncList<WebhookDeliveryEvent>({
  loader: () => inboundAutomationService.listDeliveryEvents(),
})
</script>

<template>
  <div>
    <PageHeader title="Delivery Events" description="Webhook delivery attempts and their outcomes." />
    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No delivery events recorded."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Webhook</TableHead>
                <TableHead>Recipient</TableHead>
                <TableHead>Attempt</TableHead>
                <TableHead>Response</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="e in items" :key="e.id">
                <TableCell class="whitespace-nowrap text-muted-foreground">{{ formatDateTime(e.eventTime) }}</TableCell>
                <TableCell><Badge variant="outline">{{ e.webhookName || '—' }}</Badge></TableCell>
                <TableCell>{{ e.recipient || '—' }}</TableCell>
                <TableCell class="text-muted-foreground">{{ e.attempt }}</TableCell>
                <TableCell class="font-mono text-xs">
                  {{ e.responseCode || '—' }}
                  <span v-if="e.errorSummary" class="text-muted-foreground"> · {{ e.errorSummary }}</span>
                </TableCell>
                <TableCell><StatusBadge :status="e.status" /></TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>
  </div>
</template>
