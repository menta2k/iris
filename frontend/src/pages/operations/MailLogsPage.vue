<script setup lang="ts">
import { ref } from 'vue'
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
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { usePagedList } from '@/composables/usePagedList'
import { mailOperationsService } from '@/services'
import { formatDateTime } from '@/composables/useTimezone'
import type { MailRecord, MailRecordFilters } from '@/types'

const filters = ref<MailRecordFilters>({
  mailclass: '',
  sender: '',
  from: '',
  recipient: '',
  vmta_id: '',
})

// The loader reads filters at call time, so reload() applies the current values.
const {
  items,
  loading,
  error,
  notImplemented,
  pageSize,
  pageNumber,
  hasPrev,
  hasNext,
  reload,
  nextPage,
  prevPage,
  setPageSize,
} = usePagedList<MailRecord>({
  loader: (page) => mailOperationsService.listMailRecords({ ...filters.value }, page),
})

function resetFilters() {
  filters.value = { mailclass: '', sender: '', from: '', recipient: '', vmta_id: '' }
  reload()
}
</script>

<template>
  <div>
    <PageHeader title="Mail Logs" description="Searchable record of message-level delivery events." />

    <Card class="mb-4">
      <CardContent class="p-4">
        <form class="grid items-end gap-3 md:grid-cols-6" @submit.prevent="reload">
          <div class="space-y-1">
            <Label for="f-mailclass">Mailclass</Label>
            <Input id="f-mailclass" v-model="filters.mailclass" placeholder="marketing" />
          </div>
          <div class="space-y-1">
            <Label for="f-from">From</Label>
            <Input id="f-from" v-model="filters.from" placeholder="sentry@infra.example.com" />
          </div>
          <div class="space-y-1">
            <Label for="f-sender">Sender (envelope)</Label>
            <Input id="f-sender" v-model="filters.sender" placeholder="news@example.com" />
          </div>
          <div class="space-y-1">
            <Label for="f-recipient">Recipient</Label>
            <Input id="f-recipient" v-model="filters.recipient" placeholder="user@gmail.com" />
          </div>
          <div class="space-y-1">
            <Label for="f-vmta">VMTA</Label>
            <Input id="f-vmta" v-model="filters.vmta_id" placeholder="vmta-1" />
          </div>
          <div class="flex gap-2">
            <Button type="submit" data-testid="apply-filters">Filter</Button>
            <Button type="button" variant="outline" @click="resetFilters">Reset</Button>
          </div>
        </form>
      </CardContent>
    </Card>

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No mail records match these filters."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Message ID</TableHead>
                <TableHead>Mailclass</TableHead>
                <TableHead>From</TableHead>
                <TableHead>Sender (envelope)</TableHead>
                <TableHead>Recipient</TableHead>
                <TableHead>VMTA</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Reason</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="m in items" :key="m.id">
                <TableCell class="whitespace-nowrap text-muted-foreground">{{ formatDateTime(m.eventTime) }}</TableCell>
                <TableCell class="font-mono text-xs">{{ m.messageId }}</TableCell>
                <TableCell>{{ m.mailclass }}</TableCell>
                <TableCell>{{ m.fromHeader || '—' }}</TableCell>
                <TableCell class="text-muted-foreground">{{ m.sender }}</TableCell>
                <TableCell>{{ m.recipient }}</TableCell>
                <TableCell class="font-mono text-xs">{{ m.vmtaId }}</TableCell>
                <TableCell><StatusBadge :status="m.status" /></TableCell>
                <TableCell class="max-w-md">
                  <span
                    v-if="m.smtpStatus || m.diagnostic"
                    class="font-mono text-xs text-muted-foreground"
                    :title="`${m.smtpStatus} ${m.diagnostic}`.trim()"
                  >
                    <span v-if="m.smtpStatus" class="font-semibold">{{ m.smtpStatus }}</span>
                    <span class="block truncate">{{ m.diagnostic }}</span>
                  </span>
                  <span v-else class="text-muted-foreground">—</span>
                </TableCell>
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
