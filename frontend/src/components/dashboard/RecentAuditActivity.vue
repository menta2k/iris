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
      <CardTitle class="d-flex align-center justify-space-between">
        <span>Recent Audit Activity</span>
        <span v-if="count" class="text-body-2 font-weight-regular text-medium-emphasis">{{ count }} in last hour</span>
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
            <TableCell class="text-no-wrap text-medium-emphasis">{{ formatDateTime(ev.occurredAt) }}</TableCell>
            <TableCell>{{ ev.actorUserId }}</TableCell>
            <TableCell>{{ ev.operation }}</TableCell>
            <TableCell><StatusBadge :status="ev.outcome" /></TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </CardContent>
  </Card>
</template>
