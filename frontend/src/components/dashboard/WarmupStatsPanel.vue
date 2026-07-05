<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
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
import { dashboardService, type WarmupStatsRange } from '@/services/dashboard'
import { ApiError } from '@/services/http'
import type { WarmupDeliveryStat } from '@/types'

const RANGES: WarmupStatsRange[] = ['24h', '7d']

const range = ref<WarmupStatsRange>('24h')
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)
const rows = ref<WarmupDeliveryStat[]>([])

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    const res = await dashboardService.getWarmupStats(range.value)
    rows.value = res.rows ?? []
  } catch (err) {
    rows.value = []
    if (err instanceof ApiError && err.notImplemented) {
      notImplemented.value = true
    } else {
      error.value = err instanceof Error ? err.message : 'Failed to load warmup stats.'
    }
  } finally {
    loading.value = false
  }
}

function selectRange(r: WarmupStatsRange) {
  if (r === range.value) return
  range.value = r
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
watch(range, load)
</script>

<template>
  <Card data-testid="warmup-stats-panel">
    <CardHeader class="d-flex flex-row align-center justify-space-between pb-2">
      <CardTitle class="text-body-2 text-medium-emphasis">
        Warmup delivery &amp; bounce by VMTA / domain
      </CardTitle>
      <div class="d-flex ga-1">
        <button
          v-for="r in RANGES"
          :key="r"
          type="button"
          class="rounded px-2 text-caption font-weight-medium"
          :class="r === range ? 'bg-primary' : 'text-medium-emphasis'"
          @click="selectRange(r)"
        >
          {{ r }}
        </button>
      </div>
    </CardHeader>
    <CardContent>
      <p v-if="error" class="py-6 text-center text-body-2 text-error">{{ error }}</p>
      <p v-else-if="notImplemented" class="py-6 text-center text-body-2 text-medium-emphasis">
        Warmup stats endpoint not available.
      </p>
      <p v-else-if="loading" class="py-6 text-center text-body-2 text-medium-emphasis">Loading…</p>
      <Table v-else>
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
            v-if="rows.length === 0"
            :colspan="7"
            message="No delivery activity in this range yet."
          />
          <TableRow v-for="row in rows" :key="`${row.vmtaName}|${row.recipientDomain}`">
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
    </CardContent>
  </Card>
</template>
