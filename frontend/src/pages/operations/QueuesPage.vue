<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
import ConfirmDialog from '@/components/confirm/ConfirmDialog.vue'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { mailOperationsService } from '@/services'
import { ApiError, newConfirmationId } from '@/services/http'
import { formatDateTime } from '@/composables/useTimezone'
import type { Queue, QueueAction, MailRecord } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<Queue>({
  loader: () => mailOperationsService.listQueues(),
})
const { toast } = useToast()

// "What's in the queue" — the deferred mail records (messages waiting/retrying).
// These are mail-log events, so a message can have many rows (one per retry) and
// keeps a row after it later left the queue (delivered/bounced/admin-bounced).
const deferred = ref<MailRecord[]>([])
async function loadDeferred() {
  try {
    const res = await mailOperationsService.listMailRecords({ status: 'deferred' }, { pageSize: 200 })
    deferred.value = res.items ?? []
  } catch {
    deferred.value = []
  }
}

function recipientDomain(addr?: string): string {
  const at = (addr ?? '').lastIndexOf('@')
  return at >= 0 ? addr!.slice(at + 1).toLowerCase() : ''
}

// Reflect the LIVE queue: only show deferred messages for domains kumod still has
// queued (depth > 0), and collapse each message to its most recent attempt. When a
// queue is drained (e.g. after a bounce) its domain drops out and its rows vanish.
const queued = computed<MailRecord[]>(() => {
  const live = new Set(
    items.value.filter((q) => Number(q.depth ?? 0) > 0).map((q) => q.domain.toLowerCase()),
  )
  if (live.size === 0) return []
  const seen = new Set<string>()
  const out: MailRecord[] = []
  for (const m of deferred.value) {
    if (!live.has(recipientDomain(m.recipient))) continue
    const key = m.messageId || m.id
    if (seen.has(key)) continue
    seen.add(key)
    out.push(m)
  }
  return out
})

const confirmOpen = ref(false)
const acting = ref(false)
const pending = ref<{ domain: string; action: QueueAction } | null>(null)

const actionLabels: Record<QueueAction, string> = {
  suspend: 'Suspend',
  resume: 'Resume',
  bounce: 'Bounce',
}

function requestAction(domain: string, action: QueueAction) {
  pending.value = { domain, action }
  confirmOpen.value = true
}

async function confirmAction() {
  if (!pending.value) return
  acting.value = true
  try {
    const res = await mailOperationsService.queueAction({
      action: pending.value.action,
      domain: pending.value.domain,
      // Bounce is destructive → kumod requires a confirmation id.
      confirmation_id: pending.value.action === 'bounce' ? newConfirmationId() : undefined,
    })
    toast({
      title: `${actionLabels[pending.value.action]} done`,
      description: res.summary || res.status,
      variant: 'success',
    })
    confirmOpen.value = false
    await Promise.all([load(), loadDeferred()])
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Action failed.'
    toast({ title: 'Queue action failed', description: msg, variant: 'destructive' })
  } finally {
    acting.value = false
  }
}

let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  loadDeferred()
  timer = setInterval(() => {
    load()
    loadDeferred()
  }, 15000)
})
onBeforeUnmount(() => timer && clearInterval(timer))
</script>

<template>
  <div>
    <PageHeader
      title="Queues"
      description="Live KumoMTA scheduled queues by destination domain. Suspend or resume delivery, or bounce (purge) queued mail."
    />

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No scheduled queues — nothing waiting for delivery."
    >
      <Card class="mb-6">
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead>Depth</TableHead>
                <TableHead>State</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="q in items" :key="q.domain">
                <TableCell class="font-medium">{{ q.domain }}</TableCell>
                <TableCell class="tabular-nums">{{ Number(q.depth ?? 0).toLocaleString() }}</TableCell>
                <TableCell>
                  <StatusBadge :status="q.suspended ? 'suspended' : 'running'" />
                  <span v-if="q.suspended && q.suspendReason" class="ml-2 text-xs text-muted-foreground">
                    {{ q.suspendReason }}
                  </span>
                </TableCell>
                <TableCell class="text-right">
                  <div class="flex justify-end gap-2">
                    <Button
                      v-if="!q.suspended"
                      size="sm"
                      variant="outline"
                      @click="requestAction(q.domain, 'suspend')"
                    >
                      Suspend
                    </Button>
                    <Button v-else size="sm" variant="outline" @click="requestAction(q.domain, 'resume')">
                      Resume
                    </Button>
                    <Button size="sm" variant="destructive" @click="requestAction(q.domain, 'bounce')">
                      Bounce
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>

    <Card>
      <CardHeader>
        <CardTitle class="text-sm">In the queue — deferred messages</CardTitle>
      </CardHeader>
      <CardContent class="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Time</TableHead>
              <TableHead>Recipient</TableHead>
              <TableHead>From</TableHead>
              <TableHead>Last result</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="m in queued" :key="m.id">
              <TableCell class="whitespace-nowrap text-muted-foreground">{{ formatDateTime(m.eventTime) }}</TableCell>
              <TableCell>{{ m.recipient }}</TableCell>
              <TableCell class="text-muted-foreground">{{ m.fromHeader || m.sender }}</TableCell>
              <TableCell class="max-w-md">
                <span class="font-mono text-xs">
                  <span v-if="m.smtpStatus" class="font-semibold">{{ m.smtpStatus }}</span>
                  <span class="block truncate">{{ m.diagnostic }}</span>
                </span>
              </TableCell>
            </TableRow>
            <TableRow v-if="queued.length === 0">
              <TableCell colspan="4" class="text-center text-sm text-muted-foreground">
                No messages in the queue.
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>

    <ConfirmDialog
      v-model:open="confirmOpen"
      :title="pending ? `${actionLabels[pending.action]} queue` : 'Confirm'"
      :description="
        pending
          ? pending.action === 'bounce'
            ? `This will permanently delete (bounce) all queued messages for '${pending.domain}'. This cannot be undone.`
            : `This will ${actionLabels[pending.action].toLowerCase()} delivery for the '${pending.domain}' queue.`
          : ''
      "
      :confirm-label="pending ? actionLabels[pending.action] : 'Confirm'"
      :confirm-text="pending?.action === 'bounce' ? pending.domain : undefined"
      :variant="pending?.action === 'resume' ? 'default' : 'destructive'"
      :loading="acting"
      @confirm="confirmAction"
    />
  </div>
</template>
