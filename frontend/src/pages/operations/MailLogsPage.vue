<script setup lang="ts">
import { computed, ref } from 'vue'
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
import { Select } from '@/components/ui/select'
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

// Offset-token pagination: the API returns an opaque nextPageToken; we keep a
// stack of the tokens used for prior pages so "Previous" can walk back.
const PAGE_SIZES = [25, 50, 100, 200]
const pageSize = ref('50')
const prevTokens = ref<string[]>([])
const currentToken = ref('')
const nextToken = ref('')
const pageNumber = computed(() => prevTokens.value.length + 1)
const hasNext = computed(() => nextToken.value !== '')
const hasPrev = computed(() => prevTokens.value.length > 0)

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    const res = await mailOperationsService.listMailRecords(
      { ...filters.value },
      { pageSize: Number(pageSize.value), pageToken: currentToken.value || undefined },
    )
    items.value = res.items ?? []
    nextToken.value = res.page?.nextPageToken ?? res.page?.next_page_token ?? ''
  } catch (err) {
    items.value = []
    nextToken.value = ''
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

// Any new query (filter, reset, page-size change) restarts at the first page.
function reload() {
  prevTokens.value = []
  currentToken.value = ''
  load()
}

function resetFilters() {
  filters.value = { mailclass: '', sender: '', recipient: '', vmta_id: '' }
  reload()
}

function nextPage() {
  if (!hasNext.value) return
  prevTokens.value.push(currentToken.value)
  currentToken.value = nextToken.value
  load()
}

function prevPage() {
  if (!hasPrev.value) return
  currentToken.value = prevTokens.value.pop() ?? ''
  load()
}

reload()
</script>

<template>
  <div>
    <PageHeader title="Mail Logs" description="Searchable record of message-level delivery events." />

    <Card class="mb-4">
      <CardContent class="p-4">
        <form class="grid items-end gap-3 md:grid-cols-5" @submit.prevent="reload">
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

    <!-- Pagination footer: shown whenever there are rows or earlier pages. -->
    <div
      v-if="!notImplemented && (items.length > 0 || hasPrev)"
      class="mt-3 flex items-center justify-between text-sm"
    >
      <div class="flex items-center gap-2 text-muted-foreground">
        <Label for="page-size" class="text-xs">Rows</Label>
        <Select id="page-size" v-model="pageSize" class="h-8 w-20" @change="reload">
          <option v-for="s in PAGE_SIZES" :key="s" :value="String(s)">{{ s }}</option>
        </Select>
        <span class="ml-2">Page {{ pageNumber }}</span>
      </div>
      <div class="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          :disabled="!hasPrev || loading"
          data-testid="prev-page"
          @click="prevPage"
        >
          Previous
        </Button>
        <Button
          variant="outline"
          size="sm"
          :disabled="!hasNext || loading"
          data-testid="next-page"
          @click="nextPage"
        >
          Next
        </Button>
      </div>
    </div>
  </div>
</template>
