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
  if (rate >= 0.05) return 'text-destructive font-medium'
  if (rate >= 0.02) return 'text-amber-600'
  return 'text-muted-foreground'
}

onMounted(load)
watch(range, load)
</script>

<template>
  <Card data-testid="warmup-stats-panel">
    <CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
      <CardTitle class="text-sm text-muted-foreground">
        Warmup delivery &amp; bounce by VMTA / domain
      </CardTitle>
      <div class="flex gap-1">
        <button
          v-for="r in RANGES"
          :key="r"
          type="button"
          class="rounded px-2 py-0.5 text-xs font-medium transition-colors"
          :class="
            r === range
              ? 'bg-primary text-primary-foreground'
              : 'text-muted-foreground hover:bg-muted'
          "
          @click="selectRange(r)"
        >
          {{ r }}
        </button>
      </div>
    </CardHeader>
    <CardContent>
      <p v-if="error" class="py-6 text-center text-sm text-destructive">{{ error }}</p>
      <p v-else-if="notImplemented" class="py-6 text-center text-sm text-muted-foreground">
        Warmup stats endpoint not available.
      </p>
      <p v-else-if="loading" class="py-6 text-center text-sm text-muted-foreground">Loading…</p>
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
            <TableCell class="whitespace-nowrap font-medium">{{ row.vmtaName }}</TableCell>
            <TableCell class="whitespace-nowrap">{{ row.recipientDomain }}</TableCell>
            <TableCell class="text-right tabular-nums">{{ row.sent }}</TableCell>
            <TableCell class="text-right tabular-nums">{{ row.bounced }}</TableCell>
            <TableCell class="text-right tabular-nums text-muted-foreground">
              {{ row.deferred }}
            </TableCell>
            <TableCell class="text-right tabular-nums">
              <template v-if="Number(row.attempted) > 0">{{ pct(row.deliveryRate) }}</template>
              <span v-else class="text-muted-foreground">—</span>
            </TableCell>
            <TableCell class="text-right tabular-nums" :class="bounceClass(row.bounceRate)">
              <template v-if="Number(row.attempted) > 0">{{ pct(row.bounceRate) }}</template>
              <span v-else class="text-muted-foreground">—</span>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </CardContent>
  </Card>
</template>
