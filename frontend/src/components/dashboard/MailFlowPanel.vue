<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { metricsService, type MetricsRange } from '@/services/metrics'
import { ApiError } from '@/services/http'
import type { MetricsSeries } from '@/types'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const RANGES: MetricsRange[] = ['1h', '6h', '24h', '7d']
// Stable colors per series key so a line keeps its meaning across renders.
const COLORS: Record<string, string> = {
  deliveries: '#16a34a', // green
  receptions: '#2563eb', // blue
  deferrals: '#d97706', // amber
  bounces: '#dc2626', // red
}

const range = ref<MetricsRange>('6h')
const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)
const prometheusUnavailable = ref(false)
const series = ref<MetricsSeries[]>([])

const el = ref<HTMLDivElement | null>(null)
const chart = shallowRef<echarts.ECharts | null>(null)

const hasData = computed(() => series.value.some((s) => (s.points?.length ?? 0) > 0))

function render() {
  if (!chart.value) return
  chart.value.setOption(
    {
      tooltip: { trigger: 'axis' },
      legend: { top: 0, icon: 'roundRect', textStyle: { color: '#64748b' } },
      grid: { left: 48, right: 16, top: 36, bottom: 28 },
      xAxis: {
        type: 'time',
        axisLabel: { color: '#64748b' },
        axisLine: { lineStyle: { color: '#e2e8f0' } },
      },
      yAxis: {
        type: 'value',
        min: 0,
        axisLabel: { color: '#64748b' },
        splitLine: { lineStyle: { color: '#f1f5f9' } },
      },
      series: series.value.map((s) => ({
        name: s.label,
        type: 'line',
        smooth: true,
        showSymbol: false,
        color: COLORS[s.key],
        data: (s.points ?? []).map((p) => [p.timestamp * 1000, p.value]),
      })),
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
    const res = await metricsService.getTimeseries(range.value)
    if (!res.prometheusAvailable) {
      prometheusUnavailable.value = true
      series.value = []
      return
    }
    series.value = res.series ?? []
  } catch (err) {
    series.value = []
    if (err instanceof ApiError && err.notImplemented) {
      notImplemented.value = true
    } else {
      error.value = err instanceof Error ? err.message : 'Failed to load metrics.'
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

watch(range, load)
// Re-render whenever data lands and the chart exists.
watch([series, loading], () => {
  if (hasData.value) render()
})
</script>

<template>
  <Card data-testid="mail-flow-panel">
    <CardHeader class="d-flex flex-row align-center justify-space-between pb-2">
      <CardTitle class="text-body-2 text-medium-emphasis">Mail flow (events/min)</CardTitle>
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
      <!-- The chart canvas always exists so ECharts can mount; overlays cover it. -->
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
          <span v-else>No data in this range yet.</span>
        </div>
      </div>
    </CardContent>
  </Card>
</template>
