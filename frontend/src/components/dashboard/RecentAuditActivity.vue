<script setup lang="ts">
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableEmpty,
} from '@/components/ui/table'
import { StatusBadge } from '@/components/ui/badge'
import { formatDateTime } from '@/composables/useTimezone'
import type { AuditEntry } from '@/types'

defineProps<{ events?: AuditEntry[]; count?: string }>()
</script>

<template>
  <Card data-testid="recent-audit-activity">
    <CardHeader>
      <CardTitle class="flex items-center justify-between">
        <span>Recent Audit Activity</span>
        <span v-if="count" class="text-sm font-normal text-muted-foreground">{{ count }} in last hour</span>
      </CardTitle>
    </CardHeader>
    <CardContent>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Time</TableHead>
            <TableHead>Actor</TableHead>
            <TableHead>Action</TableHead>
            <TableHead>Outcome</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableEmpty v-if="!events || events.length === 0" :colspan="4" message="No recent audit events." />
          <TableRow v-for="ev in events" :key="ev.id">
            <TableCell class="whitespace-nowrap text-muted-foreground">{{ formatDateTime(ev.occurredAt) }}</TableCell>
            <TableCell>{{ ev.actorUserId }}</TableCell>
            <TableCell>{{ ev.operation }}</TableCell>
            <TableCell><StatusBadge :status="ev.outcome" /></TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </CardContent>
  </Card>
</template>
