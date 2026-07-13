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
  // Dashboard-wide range that overrides the widget's own range when set.
  rangeOverride?: string
}>()

// The effective lookback: the dashboard-wide toggle wins over the widget's
// stored default so a single control drives every chart.
const effectiveRange = computed(() => props.rangeOverride || props.config.range)

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
const isTable = computed(() => props.config.viz === 'table')
// Table/stat widgets render HTML, not an ECharts canvas.
const isChart = computed(() => !isStat.value && !isTable.value)
const hasData = computed(() => series.value.some((s) => (s.points?.length ?? 0) > 0))

// Table rows: one per series, its latest value, ranked high→low. Useful for
// grouped metrics (by domain, by VMTA, by mount, by class).
const tableRows = computed(() => {
  const rows = series.value.map((s) => ({ label: s.label || 'value', value: lastValue(s) }))
  rows.sort((a, b) => b.value - a.value)
  return rows
})
const tableMax = computed(() => Math.max(0, ...tableRows.value.map((r) => r.value)))
function barPct(v: number): string {
  const max = tableMax.value
  return max > 0 ? `${Math.round((v / max) * 100)}%` : '0%'
}

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

const lastValue = (s: MetricsSeries): number => {
  const pts = s.points
  return pts && pts.length ? pts[pts.length - 1].value : 0
}

function render() {
  if (!chart.value || !isChart.value) return
  const t = chartTheme.value
  const isBar = props.config.viz === 'bar'
  const isArea = props.config.viz === 'area'

  // A bar chart of several series (one value per category — e.g. "disk by
  // mount", "mail by domain") reads best as a category axis: one bar per
  // series using its latest value. Instant series carry a single point at t=0,
  // which a time axis would plot at 1970 — the category axis avoids that. A
  // single-series bar keeps the time axis so it shows the value over time.
  const isCategoryBar = isBar && series.value.length > 1

  if (isCategoryBar) {
    chart.value.setOption(
      {
        tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' }, backgroundColor: t.tooltipBg, borderColor: t.tooltipBorder, textStyle: { color: t.tooltipText } },
        grid: { left: 48, right: 16, top: 12, bottom: 60 },
        xAxis: {
          type: 'category',
          data: series.value.map((s) => s.label),
          axisLabel: { color: t.axisLabel, interval: 0, rotate: series.value.length > 4 ? 40 : 0, hideOverlap: true },
          axisLine: { lineStyle: { color: t.axisLine } },
        },
        yAxis: { type: 'value', min: 0, axisLabel: { color: t.axisLabel }, splitLine: { lineStyle: { color: t.splitLine } } },
        series: [
          {
            type: 'bar',
            data: series.value.map((s, i) => ({ value: lastValue(s), itemStyle: { color: seriesColorFor(i) } })),
          },
        ],
      },
      true,
    )
    return
  }

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
      range: effectiveRange.value,
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
  if (!isChart.value) return
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
  () => [props.config.source, props.config.catalogKey, props.config.promql, effectiveRange.value, props.config.groupBy, props.refreshKey],
  load,
)
// (Re)render on data / theme / viz change; (re)create the canvas if the viz
// switched from stat to a chart type.
watch([series, chartTheme, () => props.config.viz], () => {
  if (!isChart.value) return
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
      <!-- Table widget: ranked rows (label + latest value), scrolls if tall. -->
      <div v-else-if="isTable" class="metric-widget__table h-100">
        <table class="metric-table">
          <tbody>
            <tr v-for="row in tableRows" :key="row.label">
              <td class="metric-table__label">
                <span class="metric-table__bar" :style="{ width: barPct(row.value) }" />
                <span class="metric-table__name">{{ row.label }}</span>
              </td>
              <td class="metric-table__val tabular-nums">
                {{ formatNumber(row.value) }}<span v-if="config.unit" class="text-disabled ml-1">{{ config.unit }}</span>
              </td>
            </tr>
          </tbody>
        </table>
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
.metric-widget__table {
  overflow: auto;
}
.metric-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.8125rem;
}
.metric-table td {
  padding: 4px 8px;
  border-bottom: 1px solid rgba(var(--v-border-color), 0.08);
}
.metric-table tr:last-child td {
  border-bottom: none;
}
.metric-table__label {
  position: relative;
  max-width: 0;
  width: 100%;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
/* A faint proportional bar behind each label, ranking rows visually. */
.metric-table__bar {
  position: absolute;
  left: 0;
  top: 2px;
  bottom: 2px;
  background: rgba(var(--v-theme-primary), 0.14);
  border-radius: 3px;
  z-index: 0;
}
.metric-table__name {
  position: relative;
  z-index: 1;
}
.metric-table__val {
  text-align: right;
  white-space: nowrap;
  font-weight: 600;
}
</style>
