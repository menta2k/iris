<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import RangeToggle from '@/components/dashboard/RangeToggle.vue'
import { useChartTheme } from '@/composables/useChartTheme'
import { systemMonitorService, type SystemMetricsRange } from '@/services/system-monitor'
import { ApiError } from '@/services/http'
import type { MetricsSeries } from '@/types'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const RANGES: SystemMetricsRange[] = ['1h', '6h', '24h', '7d']
// Disk series cycle a static mid-tone palette that holds up on both themes.
const DISK_PALETTE = ['#d97706', '#dc2626', '#7c3aed', '#0891b2', '#c026d3', '#65a30d']

const chartTheme = useChartTheme()

const range = ref<SystemMetricsRange>('6h')
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
  const fixed: Record<string, string> = { cpu: t.series.info, memory: t.series.success }
  let diskIndex = 0
  chart.value.setOption(
    {
      tooltip: {
        trigger: 'axis',
        backgroundColor: t.tooltipBg,
        borderColor: t.tooltipBorder,
        textStyle: { color: t.tooltipText },
        valueFormatter: (v: number) => `${Number(v).toFixed(1)}%`,
      },
      legend: { top: 0, icon: 'roundRect', textStyle: { color: t.legendText } },
      grid: { left: 44, right: 16, top: 36, bottom: 28 },
      xAxis: {
        type: 'time',
        axisLabel: { color: t.axisLabel },
        axisLine: { lineStyle: { color: t.axisLine } },
      },
      yAxis: {
        type: 'value',
        min: 0,
        max: 100,
        axisLabel: { color: t.axisLabel, formatter: '{value}%' },
        splitLine: { lineStyle: { color: t.splitLine } },
      },
      series: series.value.map((s) => ({
        name: s.label,
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2 },
        color: fixed[s.key] ?? DISK_PALETTE[(s.key.startsWith('disk:') ? diskIndex++ : 0) % DISK_PALETTE.length],
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
    const res = await systemMonitorService.getMetrics(range.value)
    if (!res.prometheusAvailable) {
      prometheusUnavailable.value = true
      series.value = []
      return
    }
    series.value = res.series ?? []
  } catch (err) {
    series.value = []
    if (err instanceof ApiError && err.notImplemented) notImplemented.value = true
    else error.value = err instanceof Error ? err.message : 'Failed to load system metrics.'
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
  <Card data-testid="system-metrics-panel">
    <CardHeader class="pb-2">
      <div class="d-flex flex-wrap align-center justify-space-between ga-2">
        <div>
          <CardTitle>Usage History</CardTitle>
          <p class="text-caption text-medium-emphasis mb-0">CPU, memory and disk over time</p>
        </div>
        <RangeToggle v-model="range" :options="RANGES" />
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
          <span v-else-if="notImplemented">System metrics endpoint not available.</span>
          <span v-else-if="prometheusUnavailable">
            No Prometheus configured — set the Prometheus URL in Settings to chart history.
          </span>
          <span v-else>No data in this range yet.</span>
        </div>
      </div>
    </CardContent>
  </Card>
</template>
