<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import RangeToggle from './RangeToggle.vue'
import { useChartTheme } from '@/composables/useChartTheme'
import { metricsService, type MetricsRange } from '@/services/metrics'
import { ApiError } from '@/services/http'
import type { MetricsSeries } from '@/types'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const RANGES: MetricsRange[] = ['1h', '6h', '24h', '7d']

const chartTheme = useChartTheme()

// Series keys map onto the app's semantic status colors so a delivery is the
// same green here as everywhere else in the UI. Both the backend's key names
// and the mock API's are listed so either data source colors correctly.
const seriesColor = computed<Record<string, string>>(() => ({
  deliveries: chartTheme.value.series.success,
  delivered: chartTheme.value.series.success,
  receptions: chartTheme.value.series.info,
  received: chartTheme.value.series.info,
  deferrals: chartTheme.value.series.warning,
  deferred: chartTheme.value.series.warning,
  bounces: chartTheme.value.series.error,
  bounced: chartTheme.value.series.error,
}))

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
  const t = chartTheme.value
  chart.value.setOption(
    {
      tooltip: {
        trigger: 'axis',
        backgroundColor: t.tooltipBg,
        borderColor: t.tooltipBorder,
        textStyle: { color: t.tooltipText },
      },
      legend: { top: 0, icon: 'roundRect', textStyle: { color: t.legendText } },
      grid: { left: 48, right: 16, top: 36, bottom: 28 },
      xAxis: {
        type: 'time',
        axisLabel: { color: t.axisLabel },
        axisLine: { lineStyle: { color: t.axisLine } },
      },
      yAxis: {
        type: 'value',
        min: 0,
        axisLabel: { color: t.axisLabel },
        splitLine: { lineStyle: { color: t.splitLine } },
      },
      series: series.value.map((s) => {
        const color = seriesColor.value[s.key] ?? t.series.primary
        return {
          name: s.label,
          type: 'line',
          smooth: true,
          showSymbol: false,
          color,
          lineStyle: { width: 2 },
          // A faint fade under each line gives the panel depth without
          // obscuring crossings.
          areaStyle: {
            opacity: 1,
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: `${color}29` },
              { offset: 1, color: `${color}00` },
            ]),
          },
          data: (s.points ?? []).map((p) => [p.timestamp * 1000, p.value]),
        }
      }),
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
// Re-render whenever data lands, and re-skin when the theme flips.
watch([series, loading, chartTheme], () => {
  if (hasData.value) render()
})
</script>

<template>
  <Card data-testid="mail-flow-panel">
    <CardHeader class="pb-2">
      <div class="d-flex flex-wrap align-center justify-space-between ga-2">
        <div>
          <CardTitle>Mail Flow</CardTitle>
          <p class="text-caption text-medium-emphasis mb-0">Events per minute</p>
        </div>
        <RangeToggle v-model="range" :options="RANGES" />
      </div>
    </CardHeader>
    <CardContent>
      <!-- The chart canvas always exists so ECharts can mount; overlays cover it. -->
      <div class="position-relative w-100" style="height: 280px">
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
