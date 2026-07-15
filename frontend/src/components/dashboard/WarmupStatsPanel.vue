<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import RangeToggle from './RangeToggle.vue'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableEmpty,
} from '@/components/ui/table'
import { dashboardService, type WarmupStatsRange } from '@/services/dashboard'
import { ApiError } from '@/services/http'
import type { DomainDeferredStat, WarmupDeliveryStat } from '@/types'

const RANGES: WarmupStatsRange[] = ['24h', '7d']

const props = defineProps<{ node?: string }>()
const range = ref<WarmupStatsRange>('24h')
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)
const rows = ref<WarmupDeliveryStat[]>([])
const deferredByDomain = ref<DomainDeferredStat[]>([])

// Cap the breakdown to the busiest domains so the panel stays readable when a
// send touches hundreds of recipient domains.
const TOP_N = 25

function rowVolume(r: WarmupDeliveryStat): number {
  return Number(r.attempted) || Number(r.sent) + Number(r.bounced)
}

// Total volume per domain, summed across VMTAs — the ranking key.
const domainVolume = computed(() => {
  const m = new Map<string, number>()
  for (const r of rows.value) {
    m.set(r.recipientDomain, (m.get(r.recipientDomain) ?? 0) + rowVolume(r))
  }
  return m
})

const topDomains = computed(() =>
  [...domainVolume.value.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, TOP_N)
    .map(([d]) => d),
)

// Rows for the top-N domains only, grouped by domain volume (busiest first).
const visibleRows = computed(() => {
  const keep = new Set(topDomains.value)
  const vol = domainVolume.value
  return rows.value
    .filter((r) => keep.has(r.recipientDomain))
    .sort(
      (a, b) =>
        (vol.get(b.recipientDomain) ?? 0) - (vol.get(a.recipientDomain) ?? 0) ||
        a.vmtaName.localeCompare(b.vmtaName),
    )
})

const topDeferred = computed(() =>
  [...deferredByDomain.value]
    .sort((a, b) => Number(b.messages) - Number(a.messages))
    .slice(0, TOP_N),
)

const hiddenDomains = computed(() => Math.max(0, domainVolume.value.size - TOP_N))

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    const res = await dashboardService.getWarmupStats(range.value, props.node ?? '')
    rows.value = res.rows ?? []
    deferredByDomain.value = res.deferredByDomain ?? []
  } catch (err) {
    rows.value = []
    deferredByDomain.value = []
    if (err instanceof ApiError && err.notImplemented) {
      notImplemented.value = true
    } else {
      error.value = err instanceof Error ? err.message : 'Failed to load warmup stats.'
    }
  } finally {
    loading.value = false
  }
}

// Percentage with one decimal, e.g. 0.9123 -> "91.2%".
function pct(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

// Color the bounce rate to flag warmup trouble at a glance.
function bounceClass(rate: number): string {
  if (rate >= 0.05) return 'text-error font-weight-medium'
  if (rate >= 0.02) return 'text-warning'
  return 'text-medium-emphasis'
}

onMounted(load)
watch([range, () => props.node], load)
</script>

<template>
  <Card data-testid="warmup-stats-panel">
    <CardHeader class="pb-2">
      <div class="d-flex flex-wrap align-center justify-space-between ga-2">
        <div>
          <CardTitle>Warmup Delivery &amp; Bounce</CardTitle>
          <p class="text-caption text-medium-emphasis mb-0">
            By VMTA / recipient domain<span v-if="hiddenDomains > 0"> · top {{ TOP_N }} domains</span>
          </p>
        </div>
        <RangeToggle v-model="range" :options="RANGES" />
      </div>
    </CardHeader>
    <CardContent>
      <p v-if="error" class="py-6 text-center text-body-2 text-error">{{ error }}</p>
      <p v-else-if="notImplemented" class="py-6 text-center text-body-2 text-medium-emphasis">
        Warmup stats endpoint not available.
      </p>
      <p v-else-if="loading" class="py-6 text-center text-body-2 text-medium-emphasis">Loading…</p>
      <template v-else>
        <!-- Distinct messages deferred per domain (deduped across VMTAs). The
             per-VMTA "Deferred" column below is per-IP incidence, so a message
             retried across IPs is counted once here but once per IP there —
             don't sum the column. -->
        <div v-if="deferredByDomain.length" class="mb-3 rounded border pa-3">
          <div class="mb-1 d-flex align-center ga-2">
            <span class="text-caption font-weight-medium">Messages deferred by domain</span>
            <span class="text-caption text-medium-emphasis">distinct — the "Deferred" column below is per IP</span>
          </div>
          <div class="d-flex flex-wrap ga-2">
            <span
              v-for="d in topDeferred"
              :key="d.recipientDomain"
              class="d-inline-flex align-center ga-1 rounded border px-2 py-1 text-caption"
            >
              <span class="font-mono">{{ d.recipientDomain }}</span>
              <span class="tabular-nums text-warning font-weight-medium">{{
                Number(d.messages).toLocaleString()
              }}</span>
            </span>
          </div>
        </div>
        <Table>
        <TableHeader>
          <TableRow>
            <TableHead>VMTA</TableHead>
            <TableHead>Domain</TableHead>
            <TableHead class="text-right">Sent</TableHead>
            <TableHead class="text-right">Bounced</TableHead>
            <TableHead class="text-right">Deferred</TableHead>
            <TableHead class="text-right">Delivery</TableHead>
            <TableHead class="text-right">Bounce</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableEmpty
            v-if="visibleRows.length === 0"
            :colspan="7"
            message="No delivery activity in this range yet."
          />
          <TableRow v-for="row in visibleRows" :key="`${row.vmtaName}|${row.recipientDomain}`">
            <TableCell class="text-no-wrap font-weight-medium">{{ row.vmtaName }}</TableCell>
            <TableCell class="text-no-wrap">{{ row.recipientDomain }}</TableCell>
            <TableCell class="text-right tabular-nums">{{ row.sent }}</TableCell>
            <TableCell class="text-right tabular-nums">{{ row.bounced }}</TableCell>
            <TableCell class="text-right tabular-nums text-medium-emphasis">
              {{ row.deferred }}
            </TableCell>
            <TableCell class="text-right tabular-nums">
              <template v-if="Number(row.attempted) > 0">{{ pct(row.deliveryRate) }}</template>
              <span v-else class="text-medium-emphasis">—</span>
            </TableCell>
            <TableCell class="text-right tabular-nums" :class="bounceClass(row.bounceRate)">
              <template v-if="Number(row.attempted) > 0">{{ pct(row.bounceRate) }}</template>
              <span v-else class="text-medium-emphasis">—</span>
            </TableCell>
          </TableRow>
        </TableBody>
        </Table>
        <p v-if="hiddenDomains > 0" class="mt-2 text-caption text-medium-emphasis">
          Showing the {{ TOP_N }} busiest domains — {{ hiddenDomains }} more hidden.
        </p>
      </template>
    </CardContent>
  </Card>
</template>
