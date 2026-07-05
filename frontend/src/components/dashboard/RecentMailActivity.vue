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
import type { MailRecord } from '@/types'

defineProps<{ events?: MailRecord[]; count?: string }>()
</script>

<template>
  <Card data-testid="recent-mail-activity">
    <CardHeader>
      <CardTitle class="d-flex align-center justify-space-between">
        <span>Recent Mail Activity</span>
        <span v-if="count" class="text-body-2 font-weight-regular text-medium-emphasis">{{ count }} in last hour</span>
      </CardTitle>
    </CardHeader>
    <CardContent>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Time</TableHead>
            <TableHead>Recipient</TableHead>
            <TableHead>Mailclass</TableHead>
            <TableHead>Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableEmpty v-if="!events || events.length === 0" :colspan="4" message="No recent mail events." />
          <TableRow v-for="ev in events" :key="ev.id">
            <TableCell class="text-no-wrap text-medium-emphasis">{{ formatDateTime(ev.eventTime) }}</TableCell>
            <TableCell>{{ ev.recipient }}</TableCell>
            <TableCell>{{ ev.mailclass }}</TableCell>
            <TableCell><StatusBadge :status="ev.status" /></TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </CardContent>
  </Card>
</template>
