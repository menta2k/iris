<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import RangeToggle from './RangeToggle.vue'
import { ApiError } from '@/services/http'
import type { WarmupStatsRange } from '@/services/dashboard'

// One ranked entry: a label (mailclass or recipient domain) with its
// mail-record counts. The parent maps a service response into this shape so the
// panel stays agnostic about which statistic it renders.
export interface VolumeRow {
  name: string
  count: number
  delivered: number
  bounced: number
  deferred: number
}

const props = defineProps<{
  title: string
  fetcher: (range: WarmupStatsRange) => Promise<VolumeRow[]>
  emptyMessage?: string
}>()

const RANGES: WarmupStatsRange[] = ['24h', '7d']

const range = ref<WarmupStatsRange>('24h')
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)
const rows = ref<VolumeRow[]>([])

// Bars are scaled to the largest count in the current set.
const maxCount = computed(() => rows.value.reduce((m, r) => Math.max(m, r.count), 0))

function barWidth(count: number): string {
  if (maxCount.value <= 0) return '0%'
  // Floor at 2% so a tiny-but-nonzero count is still visible.
  return `${Math.max(2, (count / maxCount.value) * 100)}%`
}

// Width of one outcome segment within a row's bar. Rows where no record has
// an outcome yet (all still queued) keep the neutral track only.
function segmentWidth(part: number, count: number): string {
  if (count <= 0 || part <= 0) return '0%'
  return `${(part / count) * 100}%`
}

function fmt(n: number): string {
  return n.toLocaleString()
}

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    rows.value = await props.fetcher(range.value)
  } catch (err) {
    rows.value = []
    if (err instanceof ApiError && err.notImplemented) {
      notImplemented.value = true
    } else {
      error.value = err instanceof Error ? err.message : 'Failed to load statistics.'
    }
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(range, load)
</script>

<template>
  <Card>
    <CardHeader class="pb-2">
      <div class="d-flex flex-wrap align-center justify-space-between ga-2">
        <div>
          <CardTitle>{{ title }}</CardTitle>
          <p class="text-caption text-medium-emphasis mb-0">
            <span class="d-inline-flex align-center ga-1 mr-3">
              <span class="legend-dot bg-success" />delivered
            </span>
            <span class="d-inline-flex align-center ga-1 mr-3">
              <span class="legend-dot bg-warning" />deferred
            </span>
            <span class="d-inline-flex align-center ga-1">
              <span class="legend-dot bg-error" />bounced
            </span>
          </p>
        </div>
        <RangeToggle v-model="range" :options="RANGES" />
      </div>
    </CardHeader>
    <CardContent>
      <p v-if="error" class="py-6 text-center text-body-2 text-error">{{ error }}</p>
      <p v-else-if="notImplemented" class="py-6 text-center text-body-2 text-medium-emphasis">
        Statistics endpoint not available.
      </p>
      <p v-else-if="loading" class="py-6 text-center text-body-2 text-medium-emphasis">Loading…</p>
      <p v-else-if="rows.length === 0" class="py-6 text-center text-body-2 text-medium-emphasis">
        {{ emptyMessage ?? 'No activity in this range yet.' }}
      </p>
      <div v-else class="d-flex flex-column ga-3">
        <div v-for="row in rows" :key="row.name">
          <div class="d-flex align-center justify-space-between mb-1 ga-2">
            <span class="text-body-2 font-weight-medium text-truncate" :title="row.name">
              {{ row.name }}
            </span>
            <span class="text-body-2 tabular-nums text-medium-emphasis">{{ fmt(row.count) }}</span>
          </div>
          <div
            class="volume-track"
            :title="`${fmt(row.delivered)} delivered · ${fmt(row.deferred)} deferred · ${fmt(row.bounced)} bounced`"
          >
            <div class="volume-bar" :style="{ width: barWidth(row.count) }">
              <div class="volume-segment bg-success" :style="{ width: segmentWidth(row.delivered, row.count) }" />
              <div class="volume-segment bg-warning" :style="{ width: segmentWidth(row.deferred, row.count) }" />
              <div class="volume-segment bg-error" :style="{ width: segmentWidth(row.bounced, row.count) }" />
            </div>
          </div>
        </div>
      </div>
    </CardContent>
  </Card>
</template>

<style scoped>
.volume-track {
  height: 8px;
  border-radius: 9999px;
  background: rgba(var(--v-theme-on-surface), 0.08);
  overflow: hidden;
}
.volume-bar {
  display: flex;
  gap: 2px;
  height: 100%;
  transition: width 0.3s ease;
}
.volume-segment {
  height: 100%;
  border-radius: 9999px;
  transition: width 0.3s ease;
}
.legend-dot {
  width: 8px;
  height: 8px;
  border-radius: 9999px;
  display: inline-block;
}
</style>
