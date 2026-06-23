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
      <CardTitle class="flex items-center justify-between">
        <span>Recent Mail Activity</span>
        <span v-if="count" class="text-sm font-normal text-muted-foreground">{{ count }} in last hour</span>
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
            <TableCell class="whitespace-nowrap text-muted-foreground">{{ formatDateTime(ev.eventTime) }}</TableCell>
            <TableCell>{{ ev.recipient }}</TableCell>
            <TableCell>{{ ev.mailclass }}</TableCell>
            <TableCell><StatusBadge :status="ev.status" /></TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </CardContent>
  </Card>
</template>
