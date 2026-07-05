<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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

function selectRange(r: WarmupStatsRange) {
  if (r === range.value) return
  range.value = r
}

onMounted(load)
watch(range, load)
</script>

<template>
  <Card>
    <CardHeader class="d-flex flex-row align-center justify-space-between pb-2">
      <CardTitle class="text-body-2 text-medium-emphasis">{{ title }}</CardTitle>
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
          <div class="volume-track">
            <div
              class="volume-fill"
              :style="{ width: barWidth(row.count) }"
              :title="`${fmt(row.delivered)} delivered · ${fmt(row.bounced)} bounced · ${fmt(row.deferred)} deferred`"
            />
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
.volume-fill {
  height: 100%;
  border-radius: 9999px;
  background: rgb(var(--v-theme-primary));
  transition: width 0.3s ease;
}
</style>
