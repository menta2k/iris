<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import * as echarts from 'echarts/core'
import { BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { metricsService, type MetricsRange } from '@/services/metrics'
import { ApiError } from '@/services/http'
import type { QueueTimeBucket } from '@/types'

echarts.use([BarChart, GridComponent, TooltipComponent, CanvasRenderer])

const RANGES: MetricsRange[] = ['1h', '6h', '24h', '7d']
const GLOBAL = '' // empty mailclass = global (all classes)

const range = ref<MetricsRange>('6h')
const mailclass = ref<string>(GLOBAL)
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)
const prometheusUnavailable = ref(false)
const buckets = ref<QueueTimeBucket[]>([])
const mailclasses = ref<string[]>([])
const totalCount = ref(0)

const el = ref<HTMLDivElement | null>(null)
const chart = shallowRef<echarts.ECharts | null>(null)

const hasData = computed(() => totalCount.value > 0)

const mailclassItems = computed(() => [
  { title: 'All classes', value: GLOBAL },
  ...mailclasses.value.map((m) => ({ title: m, value: m })),
])

// The numeric upper bound from the clean `le` label (robust across JSON, where
// a double +Inf can serialize as "Infinity"/null).
function leToBound(le: string): number {
  return le === '+Inf' ? Infinity : Number(le)
}

// Human-readable seconds: "480ms", "2s", "1.5m", "1h".
function fmtSeconds(s: number): string {
  if (!isFinite(s)) return '∞'
  if (s < 1) return `${Math.round(s * 1000)}ms`
  if (s < 60) return `${s % 1 === 0 ? s : s.toFixed(1)}s`
  if (s < 3600) return `${Math.round((s / 60) * 10) / 10}m`
  return `${Math.round((s / 3600) * 10) / 10}h`
}

// A bucket's x-axis label from its bounds: "≤0.5s", "0.5s–1s", ">30m".
function bucketLabel(le: string, prevBound: number): string {
  const bound = leToBound(le)
  if (!isFinite(bound)) return `>${fmtSeconds(prevBound)}`
  if (prevBound <= 0) return `≤${fmtSeconds(bound)}`
  return `${fmtSeconds(prevBound)}–${fmtSeconds(bound)}`
}

function render() {
  if (!chart.value) return
  let prev = 0
  const labels: string[] = []
  const values: number[] = []
  for (const b of buckets.value) {
    labels.push(bucketLabel(b.le, prev))
    values.push(Number(b.count))
    prev = leToBound(b.le)
  }
  chart.value.setOption(
    {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (p: { name: string; value: number }[]) =>
          `${p[0].name}<br/>${p[0].value.toLocaleString()} deliveries`,
      },
      grid: { left: 48, right: 16, top: 12, bottom: 48 },
      xAxis: {
        type: 'category',
        data: labels,
        axisLabel: { color: '#64748b', rotate: 30, fontSize: 10 },
        axisLine: { lineStyle: { color: '#e2e8f0' } },
      },
      yAxis: {
        type: 'value',
        min: 0,
        axisLabel: { color: '#64748b' },
        splitLine: { lineStyle: { color: '#f1f5f9' } },
      },
      series: [
        {
          type: 'bar',
          data: values,
          color: '#6366f1',
          barMaxWidth: 40,
          itemStyle: { borderRadius: [3, 3, 0, 0] },
        },
      ],
    },
    true,
  )
}

async function load() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  prometheusUnavailable.value = false
  try {
    const res = await metricsService.getQueueTimeHistogram(range.value, mailclass.value)
    if (!res.prometheusAvailable) {
      prometheusUnavailable.value = true
      buckets.value = []
      totalCount.value = 0
      return
    }
    buckets.value = res.buckets ?? []
    // Keep the selector list stable (the backend returns it regardless of filter).
    if (res.mailclasses?.length) mailclasses.value = res.mailclasses
    totalCount.value = Number(res.totalCount) || 0
  } catch (err) {
    buckets.value = []
    totalCount.value = 0
    if (err instanceof ApiError && err.notImplemented) {
      notImplemented.value = true
    } else {
      error.value = err instanceof Error ? err.message : 'Failed to load queue-time histogram.'
    }
  } finally {
    loading.value = false
  }
}

function selectRange(r: MetricsRange) {
  if (r === range.value) return
  range.value = r
}

const onResize = () => chart.value?.resize()

onMounted(() => {
  if (el.value) {
    chart.value = echarts.init(el.value)
    window.addEventListener('resize', onResize)
  }
  load()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  chart.value?.dispose()
  chart.value = null
})

watch([range, mailclass], load)
watch([buckets, loading], () => {
  if (hasData.value) render()
})
</script>

<template>
  <Card>
    <CardHeader class="d-flex flex-row align-center justify-space-between pb-2 ga-2">
      <CardTitle class="text-body-2 text-medium-emphasis">
        Delivery queue time
        <span v-if="hasData" class="ml-1">· {{ totalCount.toLocaleString() }} delivered</span>
      </CardTitle>
      <div class="d-flex align-center ga-2">
        <v-select
          v-model="mailclass"
          :items="mailclassItems"
          density="compact"
          variant="outlined"
          hide-details
          style="min-width: 150px"
        />
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
      </div>
    </CardHeader>
    <CardContent>
      <div class="position-relative w-100" style="height: 256px">
        <div ref="el" class="h-100 w-100" />
        <div
          v-if="loading || error || notImplemented || prometheusUnavailable || !hasData"
          class="position-absolute top-0 left-0 right-0 bottom-0 d-flex align-center justify-center text-center text-body-2 text-medium-emphasis"
        >
          <span v-if="loading">Loading…</span>
          <span v-else-if="error" class="text-error">{{ error }}</span>
          <span v-else-if="notImplemented">Metrics endpoint not available.</span>
          <span v-else-if="prometheusUnavailable">
            No Prometheus configured. Set the Prometheus URL in
            <RouterLink to="/settings" class="text-decoration-underline">Settings</RouterLink>.
          </span>
          <span v-else>No deliveries in this range yet.</span>
        </div>
      </div>
    </CardContent>
  </Card>
</template>
