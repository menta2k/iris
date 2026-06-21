<script setup lang="ts">
import { ref } from 'vue'
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
import { StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { mailOperationsService } from '@/services'
import { ApiError } from '@/services/http'
import type { MailRecord, MailRecordFilters } from '@/types'

const items = ref<MailRecord[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)

const filters = ref<MailRecordFilters>({
  mailclass: '',
  sender: '',
  recipient: '',
  vmta_id: '',
})

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    const res = await mailOperationsService.listMailRecords({ ...filters.value })
    items.value = res.items ?? []
  } catch (err) {
    items.value = []
    if (err instanceof ApiError && err.notImplemented) {
      notImplemented.value = true
    } else if (err instanceof ApiError && err.status === 0) {
      error.value = 'Cannot reach the backend. Is the API server running?'
    } else {
      error.value = err instanceof Error ? err.message : 'Failed to load mail logs.'
    }
  } finally {
    loading.value = false
  }
}

function resetFilters() {
  filters.value = { mailclass: '', sender: '', recipient: '', vmta_id: '' }
  load()
}

load()
</script>

<template>
  <div>
    <PageHeader title="Mail Logs" description="Searchable record of message-level delivery events." />

    <Card class="mb-4">
      <CardContent class="p-4">
        <form class="grid items-end gap-3 md:grid-cols-5" @submit.prevent="load">
          <div class="space-y-1">
            <Label for="f-mailclass">Mailclass</Label>
            <Input id="f-mailclass" v-model="filters.mailclass" placeholder="marketing" />
          </div>
          <div class="space-y-1">
            <Label for="f-sender">Sender</Label>
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
                <TableHead>Sender</TableHead>
                <TableHead>Recipient</TableHead>
                <TableHead>VMTA</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="m in items" :key="m.id">
                <TableCell class="whitespace-nowrap text-muted-foreground">{{ m.eventTime }}</TableCell>
                <TableCell class="font-mono text-xs">{{ m.messageId }}</TableCell>
                <TableCell>{{ m.mailclass }}</TableCell>
                <TableCell>{{ m.sender }}</TableCell>
                <TableCell>{{ m.recipient }}</TableCell>
                <TableCell class="font-mono text-xs">{{ m.vmtaId }}</TableCell>
                <TableCell><StatusBadge :status="m.status" /></TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>
  </div>
</template>
