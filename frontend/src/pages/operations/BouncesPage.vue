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
import { Badge, StatusBadge } from '@/components/ui/badge'
import { useAsyncList } from '@/composables/useAsyncList'
import { mailOperationsService } from '@/services'
import type { Bounce } from '@/types'

const { items, loading, error, notImplemented } = useAsyncList<Bounce>({
  loader: () => mailOperationsService.listBounces(),
})
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
  </div>
</template>
