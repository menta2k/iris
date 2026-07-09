<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import DataState from '@/components/common/DataState.vue'
import StatTile from '@/components/dashboard/StatTile.vue'
import { Card, CardContent } from '@/components/ui/card'
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
import ConfirmDialog from '@/components/confirm/ConfirmDialog.vue'
import { useAsyncList } from '@/composables/useAsyncList'
import { useToast } from '@/composables/useToast'
import { mailOperationsService } from '@/services'
import { ApiError, newConfirmationId } from '@/services/http'
import { formatDateTime } from '@/composables/useTimezone'
import type { Queue, QueueAction, MailRecord, NextDeliveryAttempt } from '@/types'

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

// ---- Queue table: search, ordering, drill-down selection ----

const queueSearch = ref('')

// Suspended queues need attention first, then the deepest backlogs.
const sortedQueues = computed(() => {
  const term = (queueSearch.value ?? '').trim().toLowerCase()
  return items.value
    .filter((q) => !term || q.domain.toLowerCase().includes(term))
    .slice()
    .sort(
      (a, b) =>
        Number(b.suspended) - Number(a.suspended) ||
        Number(b.depth ?? 0) - Number(a.depth ?? 0) ||
        a.domain.localeCompare(b.domain),
    )
})

const maxDepth = computed(() =>
  items.value.reduce((m, q) => Math.max(m, Number(q.depth ?? 0)), 0),
)

function depthWidth(depth: string | number | undefined): string {
  const n = Number(depth ?? 0)
  if (maxDepth.value <= 0 || n <= 0) return '0%'
  return `${Math.max(3, (n / maxDepth.value) * 100)}%`
}

// Clicking a queue row narrows the deferred table to that destination domain.
const selectedDomain = ref<string | null>(null)

function toggleDomain(domain: string) {
  selectedDomain.value = selectedDomain.value === domain ? null : domain
}

const visibleQueued = computed(() =>
  selectedDomain.value
    ? queued.value.filter((m) => recipientDomain(m.recipient) === selectedDomain.value)
    : queued.value,
)

// ---- KPI tiles ----

const totalDepth = computed(() =>
  items.value.reduce((sum, q) => sum + Number(q.depth ?? 0), 0),
)
const suspendedCount = computed(() => items.value.filter((q) => q.suspended).length)
const activeCount = computed(() => items.value.length - suspendedCount.value)

// ---- Queue actions (suspend / resume / bounce with confirmation) ----

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
    await refreshAll()
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Action failed.'
    toast({ title: 'Queue action failed', description: msg, variant: 'destructive' })
  } finally {
    acting.value = false
  }
}

// ---- Deferred rows: expandable retry-schedule estimate ----

const expandedId = ref<string | null>(null)

type AttemptState = { loading: boolean; error: string | null; estimate: NextDeliveryAttempt | null }
const attemptByMessage = ref<Record<string, AttemptState>>({})

async function loadEstimate(m: MailRecord) {
  if (!m.messageId || attemptByMessage.value[m.messageId]) return // already loaded/loading
  attemptByMessage.value = {
    ...attemptByMessage.value,
    [m.messageId]: { loading: true, error: null, estimate: null },
  }
  try {
    const est = await mailOperationsService.nextDeliveryAttempt(m.messageId)
    attemptByMessage.value = {
      ...attemptByMessage.value,
      [m.messageId]: { loading: false, error: null, estimate: est },
    }
  } catch (err) {
    const msg = err instanceof ApiError ? err.message : 'Failed to estimate the retry schedule.'
    attemptByMessage.value = {
      ...attemptByMessage.value,
      [m.messageId]: { loading: false, error: msg, estimate: null },
    }
  }
}

function toggleExpand(m: MailRecord) {
  expandedId.value = expandedId.value === m.id ? null : m.id
  if (expandedId.value === m.id) loadEstimate(m)
}

// ---- Live refresh (default on — this is a live operational view) ----

const REFRESH_MS = 15_000

const live = ref(true)
const lastUpdated = ref<Date | null>(null)
let timer: ReturnType<typeof setInterval> | undefined

async function refreshAll() {
  await Promise.all([load(), loadDeferred()])
  lastUpdated.value = new Date()
}

watch(
  live,
  (on) => {
    clearInterval(timer)
    if (on) timer = setInterval(refreshAll, REFRESH_MS)
  },
  { immediate: true },
)

onMounted(() => {
  loadDeferred()
})
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <div>
    <PageHeader
      title="Queues"
      description="Live KumoMTA scheduled queues by destination domain. Suspend or resume delivery, or bounce (purge) queued mail."
    />

    <v-row dense class="mb-2">
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="Queued Messages"
          :value="totalDepth.toLocaleString()"
          caption="Across all scheduled queues"
          icon="mdi-email-multiple-outline"
          color="primary"
        />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="Active Queues"
          :value="activeCount.toLocaleString()"
          caption="Delivering normally"
          icon="mdi-truck-fast-outline"
          color="success"
        />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="Suspended Queues"
          :value="suspendedCount.toLocaleString()"
          caption="Delivery paused"
          icon="mdi-pause-circle-outline"
          :color="suspendedCount > 0 ? 'warning' : 'secondary'"
          :value-class="suspendedCount > 0 ? 'text-warning' : ''"
        />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="Deferred Messages"
          :value="queued.length.toLocaleString()"
          caption="Waiting for a retry"
          icon="mdi-clock-alert-outline"
          :color="queued.length > 0 ? 'info' : 'secondary'"
        />
      </v-col>
    </v-row>

    <Card class="mb-6">
      <div class="d-flex flex-wrap align-center ga-2 px-4 py-2">
        <div class="mr-1">
          <span class="text-subtitle-1 font-weight-bold">Scheduled Queues</span>
          <p class="text-caption text-medium-emphasis mb-0">
            <template v-if="lastUpdated">Updated {{ lastUpdated.toLocaleTimeString() }}</template>
            <template v-else>Click a row to inspect its deferred mail</template>
          </p>
        </div>
        <v-spacer />
        <v-text-field
          v-model="queueSearch"
          placeholder="Search domain"
          prepend-inner-icon="mdi-magnify"
          variant="outlined"
          density="compact"
          hide-details
          clearable
          class="flex-grow-0"
          style="width: 220px"
        />
        <v-switch
          v-model="live"
          label="Live"
          color="primary"
          density="compact"
          hide-details
          class="mr-1 flex-grow-0"
        />
        <v-btn
          icon="mdi-refresh"
          variant="text"
          size="small"
          :loading="loading"
          aria-label="Refresh"
          title="Refresh"
          @click="refreshAll"
        />
      </div>
      <v-divider />
      <v-progress-linear :active="loading" indeterminate color="primary" height="2" />
      <CardContent class="pa-0">
        <!-- Keep the table mounted during the 15s live refresh cycle. -->
        <DataState
          :loading="loading && items.length === 0"
          :error="error"
          :not-implemented="notImplemented"
          :empty="items.length === 0"
          empty-message="No scheduled queues — nothing waiting for delivery."
        >
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead style="width: 260px">Depth</TableHead>
                <TableHead>State</TableHead>
                <TableHead class="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableEmpty
                v-if="sortedQueues.length === 0"
                :colspan="4"
                message="No queues match the search."
              />
              <TableRow
                v-for="q in sortedQueues"
                :key="q.domain"
                :class="selectedDomain === q.domain ? 'row-clickable row-selected' : 'row-clickable'"
                :title="selectedDomain === q.domain ? 'Clear the deferred-mail filter' : `Show deferred mail for ${q.domain}`"
                @click="toggleDomain(q.domain)"
              >
                <TableCell class="font-weight-medium">{{ q.domain }}</TableCell>
                <TableCell>
                  <div class="d-flex align-center ga-2">
                    <span class="tabular-nums" style="min-width: 56px">
                      {{ Number(q.depth ?? 0).toLocaleString() }}
                    </span>
                    <div class="depth-track flex-grow-1">
                      <div
                        class="depth-fill"
                        :class="q.suspended ? 'bg-warning' : 'bg-primary'"
                        :style="{ width: depthWidth(q.depth) }"
                      />
                    </div>
                  </div>
                </TableCell>
                <TableCell>
                  <StatusBadge :status="q.suspended ? 'suspended' : 'running'" />
                  <span v-if="q.suspended && q.suspendReason" class="ml-2 text-caption text-medium-emphasis">
                    {{ q.suspendReason }}
                  </span>
                </TableCell>
                <TableCell class="text-right">
                  <div class="d-flex justify-end ga-1">
                    <v-btn
                      v-if="!q.suspended"
                      icon="mdi-pause"
                      variant="text"
                      size="small"
                      color="warning"
                      aria-label="Suspend delivery"
                      title="Suspend delivery"
                      @click.stop="requestAction(q.domain, 'suspend')"
                    />
                    <v-btn
                      v-else
                      icon="mdi-play"
                      variant="text"
                      size="small"
                      color="success"
                      aria-label="Resume delivery"
                      title="Resume delivery"
                      @click.stop="requestAction(q.domain, 'resume')"
                    />
                    <v-btn
                      icon="mdi-email-remove-outline"
                      variant="text"
                      size="small"
                      color="error"
                      aria-label="Bounce (purge) queued mail"
                      title="Bounce (purge) queued mail"
                      @click.stop="requestAction(q.domain, 'bounce')"
                    />
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </DataState>
      </CardContent>
    </Card>

    <Card>
      <div class="d-flex flex-wrap align-center ga-2 px-4 py-2">
        <div>
          <span class="text-subtitle-1 font-weight-bold">Deferred Messages</span>
          <p class="text-caption text-medium-emphasis mb-0">
            Latest attempt per message still in the queue · expand a row for its retry schedule
          </p>
        </div>
        <v-chip
          v-if="selectedDomain"
          closable
          size="small"
          color="primary"
          variant="tonal"
          class="ml-1"
          @click:close="selectedDomain = null"
        >
          {{ selectedDomain }}
        </v-chip>
        <v-spacer />
        <span class="text-caption text-medium-emphasis">{{ visibleQueued.length }} messages</span>
      </div>
      <v-divider />
      <CardContent class="pa-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead style="width: 40px" />
              <TableHead>Time</TableHead>
              <TableHead>Recipient</TableHead>
              <TableHead>From</TableHead>
              <TableHead>Last result</TableHead>
              <TableHead style="width: 56px" />
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableEmpty
              v-if="visibleQueued.length === 0"
              :colspan="6"
              :message="selectedDomain ? `No deferred messages for ${selectedDomain}.` : 'No messages in the queue.'"
            />
            <template v-for="m in visibleQueued" :key="m.id">
              <TableRow class="row-clickable" @click="toggleExpand(m)">
                <TableCell>
                  <v-icon
                    size="small"
                    icon="mdi-chevron-down"
                    class="expand-icon"
                    :class="expandedId === m.id ? 'expand-icon--open' : ''"
                  />
                </TableCell>
                <TableCell class="text-no-wrap text-medium-emphasis">{{ formatDateTime(m.eventTime) }}</TableCell>
                <TableCell>{{ m.recipient }}</TableCell>
                <TableCell class="text-medium-emphasis">{{ m.fromHeader || m.sender }}</TableCell>
                <TableCell style="max-width: 448px">
                  <span class="font-mono text-caption">
                    <span v-if="m.smtpStatus" class="font-weight-bold">{{ m.smtpStatus }}</span>
                    <span class="d-block text-truncate">{{ m.diagnostic }}</span>
                  </span>
                </TableCell>
                <TableCell class="text-right">
                  <v-btn
                    icon="mdi-open-in-new"
                    variant="text"
                    size="x-small"
                    aria-label="Open in Mail Logs"
                    title="Open in Mail Logs"
                    :to="{ path: '/operations/mail-logs', query: { record: m.id } }"
                    @click.stop
                  />
                </TableCell>
              </TableRow>
              <tr v-if="expandedId === m.id">
                <td :colspan="6" class="px-4 py-3">
                  <p class="mb-2 text-caption text-uppercase text-medium-emphasis">Retry schedule (estimated)</p>
                  <template v-if="m.messageId && attemptByMessage[m.messageId]">
                    <div v-if="attemptByMessage[m.messageId].loading" class="text-caption text-medium-emphasis">
                      Estimating…
                    </div>
                    <div v-else-if="attemptByMessage[m.messageId].error" class="text-caption text-error">
                      {{ attemptByMessage[m.messageId].error }}
                    </div>
                    <template v-else-if="attemptByMessage[m.messageId].estimate">
                      <div
                        v-if="!attemptByMessage[m.messageId].estimate!.deferred"
                        class="text-caption text-medium-emphasis"
                      >
                        This message is no longer deferred — it has left the queue.
                      </div>
                      <div v-else class="d-flex flex-wrap ga-6">
                        <div>
                          <p class="text-caption text-medium-emphasis mb-0">Attempts</p>
                          <p class="text-body-2 font-weight-medium tabular-nums mb-0">
                            {{ attemptByMessage[m.messageId].estimate!.attempts }}
                            <span class="text-medium-emphasis font-weight-regular">
                              · {{ attemptByMessage[m.messageId].estimate!.remainingAttempts }} remaining
                            </span>
                          </p>
                        </div>
                        <div v-if="attemptByMessage[m.messageId].estimate!.nextAttempt">
                          <p class="text-caption text-medium-emphasis mb-0">Next attempt</p>
                          <p class="text-body-2 font-weight-medium mb-0">
                            {{ formatDateTime(attemptByMessage[m.messageId].estimate!.nextAttempt) }}
                            <span
                              v-if="attemptByMessage[m.messageId].estimate!.interval"
                              class="text-medium-emphasis font-weight-regular"
                            >
                              · every {{ attemptByMessage[m.messageId].estimate!.interval }}
                            </span>
                          </p>
                        </div>
                        <div v-if="attemptByMessage[m.messageId].estimate!.finalAttempt">
                          <p class="text-caption text-medium-emphasis mb-0">Final attempt</p>
                          <p class="text-body-2 font-weight-medium mb-0">
                            {{ formatDateTime(attemptByMessage[m.messageId].estimate!.finalAttempt) }}
                          </p>
                        </div>
                        <div v-if="attemptByMessage[m.messageId].estimate!.expiresAt">
                          <p class="text-caption text-medium-emphasis mb-0">Expires</p>
                          <p
                            class="text-body-2 font-weight-medium mb-0"
                            :class="attemptByMessage[m.messageId].estimate!.willExpire ? 'text-warning' : ''"
                          >
                            {{ formatDateTime(attemptByMessage[m.messageId].estimate!.expiresAt) }}
                          </p>
                        </div>
                      </div>
                    </template>
                  </template>
                  <div v-else class="text-caption text-medium-emphasis">
                    No message id on this record — cannot estimate the schedule.
                  </div>
                </td>
              </tr>
            </template>
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

<style scoped>
.row-clickable {
  cursor: pointer;
}
.row-selected {
  background: rgba(var(--v-theme-primary), 0.08);
}

.expand-icon {
  transition: transform 0.2s ease;
}
.expand-icon--open {
  transform: rotate(180deg);
}

.depth-track {
  height: 6px;
  max-width: 160px;
  border-radius: 9999px;
  background: rgba(var(--v-theme-on-surface), 0.08);
  overflow: hidden;
}
.depth-fill {
  height: 100%;
  border-radius: 9999px;
  transition: width 0.3s ease;
}
</style>
