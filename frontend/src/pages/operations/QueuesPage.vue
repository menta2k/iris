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
import ConfirmDialog from '@/components/confirm/ConfirmDialog.vue'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { mailOperationsService } from '@/services'
import { ApiError, newConfirmationId } from '@/services/http'
import type { Queue, QueueAction } from '@/types'

const { items, loading, error, notImplemented, load } = useAsyncList<Queue>({
  loader: () => mailOperationsService.listQueues(),
})
const { toast } = useToast()

const confirmOpen = ref(false)
const acting = ref(false)
const pending = ref<{ mailclass: string; action: QueueAction } | null>(null)

const actionLabels: Record<QueueAction, string> = {
  pause: 'Pause',
  resume: 'Resume',
  drain: 'Drain',
  flush: 'Flush',
}

function formatAge(value?: string | number): string {
  const seconds = Number(value ?? 0)
  if (!seconds || seconds <= 0) return '—'
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`
  return `${Math.floor(seconds / 86400)}d`
}

function requestAction(mailclass: string, action: QueueAction) {
  pending.value = { mailclass, action }
  confirmOpen.value = true
}

async function confirmAction() {
  if (!pending.value) return
  acting.value = true
  try {
    const res = await mailOperationsService.queueAction(pending.value.mailclass, {
      action: pending.value.action,
      confirmation_id: newConfirmationId(),
    })
    toast({
      title: `${actionLabels[pending.value.action]} requested`,
      description: `Request ${res.request_id} — ${res.status}`,
      variant: 'success',
    })
    confirmOpen.value = false
    await load()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Action failed.'
    toast({ title: 'Queue action failed', description: msg, variant: 'destructive' })
  } finally {
    acting.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="Queues" description="Per-mailclass spool queues and their delivery state." />

    <DataState
      :loading="loading"
      :error="error"
      :not-implemented="notImplemented"
      :empty="items.length === 0"
      empty-message="No active queues."
    >
      <Card>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Mailclass</TableHead>
                <TableHead>Depth</TableHead>
                <TableHead>Oldest Message</TableHead>
                <TableHead>State</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="q in items" :key="q.mailclass">
                <TableCell class="font-medium">{{ q.mailclass }}</TableCell>
                <TableCell class="tabular-nums">{{ Number(q.depth ?? 0).toLocaleString() }}</TableCell>
                <TableCell class="tabular-nums text-muted-foreground">{{ formatAge(q.oldestMessageAgeSeconds) }}</TableCell>
                <TableCell><StatusBadge :status="q.state" /></TableCell>
                <TableCell class="text-right">
                  <div class="flex justify-end gap-2">
                    <Button size="sm" variant="outline" @click="requestAction(q.mailclass, 'pause')">
                      Pause
                    </Button>
                    <Button size="sm" variant="outline" @click="requestAction(q.mailclass, 'resume')">
                      Resume
                    </Button>
                    <Button size="sm" variant="destructive" @click="requestAction(q.mailclass, 'drain')">
                      Drain
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </DataState>

    <ConfirmDialog
      v-model:open="confirmOpen"
      :title="pending ? `${actionLabels[pending.action]} queue` : 'Confirm'"
      :description="
        pending
          ? `This will ${actionLabels[pending.action].toLowerCase()} the '${pending.mailclass}' queue. This affects live delivery.`
          : ''
      "
      :confirm-label="pending ? actionLabels[pending.action] : 'Confirm'"
      :confirm-text="pending?.action === 'drain' ? pending.mailclass : undefined"
      :variant="pending?.action === 'resume' ? 'default' : 'destructive'"
      :loading="acting"
      @confirm="confirmAction"
    />
  </div>
</template>
