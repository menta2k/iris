<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useChartTheme } from '@/composables/useChartTheme'
import { metricsService } from '@/services/metrics'
import { ApiError } from '@/services/http'
import type { MetricsSeries, WidgetConfig } from '@/types'

echarts.use([LineChart, BarChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const props = defineProps<{
  config: WidgetConfig
  // Bumped by the parent to trigger a coordinated refresh (SSE tick / interval).
  refreshKey?: number
}>()

const emit = defineEmits<{
  (e: 'edit'): void
  (e: 'remove'): void
}>()

const chartTheme = useChartTheme()

const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)
const prometheusUnavailable = ref(false)
const series = ref<MetricsSeries[]>([])

const el = ref<HTMLDivElement | null>(null)
const chart = shallowRef<echarts.ECharts | null>(null)
let resizeObserver: ResizeObserver | null = null

const isStat = computed(() => props.config.viz === 'stat' || props.config.viz === 'gauge')
const hasData = computed(() => series.value.some((s) => (s.points?.length ?? 0) > 0))

// Single-value widgets show the latest point of the first series.
const statValue = computed(() => {
  const pts = series.value[0]?.points
  if (!pts || pts.length === 0) return null
  return pts[pts.length - 1].value
})

const formattedStat = computed(() => {
  const v = statValue.value
  if (v === null) return '—'
  return formatNumber(v)
})

function formatNumber(v: number): string {
  if (Math.abs(v) >= 1000) return v.toLocaleString(undefined, { maximumFractionDigits: 0 })
  if (Number.isInteger(v)) return String(v)
  return v.toFixed(2)
}

const seriesColorFor = (i: number): string => {
  const t = chartTheme.value.series
  const palette = [t.primary, t.info, t.success, t.warning, t.error]
  return palette[i % palette.length]
}

function render() {
  if (!chart.value || isStat.value) return
  const t = chartTheme.value
  const isBar = props.config.viz === 'bar'
  const isArea = props.config.viz === 'area'
  chart.value.setOption(
    {
      tooltip: {
        trigger: 'axis',
        backgroundColor: t.tooltipBg,
        borderColor: t.tooltipBorder,
        textStyle: { color: t.tooltipText },
      },
      legend:
        series.value.length > 1
          ? { top: 0, icon: 'roundRect', textStyle: { color: t.legendText }, type: 'scroll' }
          : undefined,
      grid: { left: 48, right: 16, top: series.value.length > 1 ? 32 : 12, bottom: 24 },
      xAxis: {
        type: 'time',
        axisLabel: { color: t.axisLabel, hideOverlap: true },
        axisLine: { lineStyle: { color: t.axisLine } },
      },
      yAxis: {
        type: 'value',
        min: 0,
        axisLabel: { color: t.axisLabel },
        splitLine: { lineStyle: { color: t.splitLine } },
      },
      series: series.value.map((s, i) => {
        const color = seriesColorFor(i)
        const data = (s.points ?? []).map((p) => [p.timestamp * 1000, p.value])
        if (isBar) {
          return { name: s.label, type: 'bar', color, data }
        }
        return {
          name: s.label,
          type: 'line',
          smooth: true,
          showSymbol: false,
          color,
          lineStyle: { width: 2 },
          areaStyle: isArea
            ? {
                opacity: 1,
                color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                  { offset: 0, color: `${color}33` },
                  { offset: 1, color: `${color}00` },
                ]),
              }
            : undefined,
          data,
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
    const res = await metricsService.getWidgetData({
      source: props.config.source,
      catalogKey: props.config.catalogKey,
      promql: props.config.promql,
      range: props.config.range,
      groupBy: props.config.groupBy,
    })
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
      error.value = err instanceof Error ? err.message : 'Failed to load metric.'
    }
  } finally {
    loading.value = false
  }
}

function ensureChart() {
  if (isStat.value) return
  if (!chart.value && el.value) {
    chart.value = echarts.init(el.value)
    resizeObserver = new ResizeObserver(() => chart.value?.resize())
    resizeObserver.observe(el.value)
  }
}

onMounted(() => {
  ensureChart()
  load()
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
  chart.value?.dispose()
  chart.value = null
})

// Reload when the widget's query config changes or the parent forces a refresh.
watch(
  () => [props.config.source, props.config.catalogKey, props.config.promql, props.config.range, props.config.groupBy, props.refreshKey],
  load,
)
// (Re)render on data / theme / viz change; (re)create the canvas if the viz
// switched from stat to a chart type.
watch([series, chartTheme, () => props.config.viz], () => {
  if (isStat.value) return
  ensureChart()
  if (hasData.value) render()
})
</script>

<template>
  <v-card class="metric-widget d-flex flex-column h-100" elevation="1">
    <div class="d-flex align-center justify-space-between px-3 py-2 metric-widget__header">
      <div class="text-body-2 font-weight-medium text-truncate">{{ config.title }}</div>
      <div class="d-flex align-center ga-1">
        <v-btn
          icon="mdi-pencil-outline"
          size="x-small"
          variant="text"
          density="comfortable"
          :aria-label="`Edit ${config.title}`"
          @click="emit('edit')"
        />
        <v-btn
          icon="mdi-close"
          size="x-small"
          variant="text"
          density="comfortable"
          :aria-label="`Remove ${config.title}`"
          @click="emit('remove')"
        />
      </div>
    </div>
    <v-divider />

    <div class="flex-grow-1 position-relative pa-2" style="min-height: 0">
      <!-- Single-value widget -->
      <div v-if="isStat" class="d-flex flex-column align-center justify-center h-100">
        <span class="text-h4 font-weight-bold tabular-nums">{{ formattedStat }}</span>
        <span v-if="config.unit" class="text-caption text-medium-emphasis mt-1">{{ config.unit }}</span>
      </div>
      <!-- Chart widget: canvas always present so ECharts can mount. -->
      <div v-else ref="el" class="h-100 w-100" />

      <div
        v-if="loading || error || notImplemented || prometheusUnavailable || !hasData"
        class="position-absolute top-0 left-0 right-0 bottom-0 d-flex align-center justify-center text-center text-caption text-medium-emphasis pa-3"
      >
        <span v-if="loading">Loading…</span>
        <span v-else-if="error" class="text-error">{{ error }}</span>
        <span v-else-if="notImplemented">Metrics endpoint not available.</span>
        <span v-else-if="prometheusUnavailable">No Prometheus configured.</span>
        <span v-else-if="!isStat">No data in this range yet.</span>
      </div>
    </div>
  </v-card>
</template>

<style scoped>
.metric-widget {
  overflow: hidden;
}
.metric-widget__header {
  cursor: inherit;
}
</style>
