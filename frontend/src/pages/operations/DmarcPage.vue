<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import * as echarts from 'echarts/core'
import { BarChart, LineChart, PieChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import PageHeader from '@/components/common/PageHeader.vue'
import PaginationControls from '@/components/common/PaginationControls.vue'
import StatTile from '@/components/dashboard/StatTile.vue'
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
import { Badge } from '@/components/ui/badge'
import { useToast } from '@/composables/useToast'
import { useChartTheme } from '@/composables/useChartTheme'
import { usePagedList } from '@/composables/usePagedList'
import { formatDateTime } from '@/composables/useTimezone'
import { dmarcService } from '@/services'
import { ApiError } from '@/services/http'
import type { DmarcReport, DmarcStats } from '@/types'

echarts.use([BarChart, LineChart, PieChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const { toast } = useToast()
const chartTheme = useChartTheme()

// Quick relative windows sent as the stats API's `from` lower bound.
const TIME_ITEMS = [
  { title: 'All time', value: 0 },
  { title: 'Last 7 days', value: 7 * 24 * 60 * 60_000 },
  { title: 'Last 30 days', value: 30 * 24 * 60 * 60_000 },
  { title: 'Last 90 days', value: 90 * 24 * 60 * 60_000 },
]

const domains = ref<string[]>([])
const domain = ref('')
const reporter = ref('')
const timeWindowMs = ref(0)
const loading = ref(false)
const stats = ref<DmarcStats | null>(null)

const pct = (n: number, total: number) => (total > 0 ? Math.round((n / total) * 1000) / 10 : 0)
const total = computed(() => stats.value?.totalMessages ?? 0)

// Alignment health thresholds: a little failure is normal (forwarding,
// mailing lists), sustained failure is a deliverability problem.
function passClass(rate: number): string {
  if (total.value === 0) return ''
  if (rate >= 95) return 'text-success'
  if (rate >= 85) return 'text-warning'
  return 'text-error'
}

const dmarcRate = computed(() => pct(stats.value?.dmarcPass ?? 0, total.value))
const spfRate = computed(() => pct(stats.value?.spfPass ?? 0, total.value))
const dkimRate = computed(() => pct(stats.value?.dkimPass ?? 0, total.value))

const hasActiveFilters = computed(
  () => domain.value !== '' || reporter.value !== '' || timeWindowMs.value > 0,
)

function resetFilters() {
  domain.value = ''
  reporter.value = ''
  timeWindowMs.value = 0
}

const domainItems = computed(() => [
  { title: 'All domains', value: '' },
  ...domains.value.map((d) => ({ title: d, value: d })),
])

// Reporter drill-down. The reporters breakdown ignores the reporter filter, so
// this list stays complete even while one reporter is selected.
const reporterItems = computed(() => [
  { title: 'All reporters', value: '' },
  ...(stats.value?.reporters ?? []).map((r) => ({ title: r.reporter, value: r.reporter })),
])

// Toggle a reporter drill-down from the "By reporter" table.
function selectReporter(name: string) {
  reporter.value = reporter.value === name ? '' : name
}

const dispEl = ref<HTMLDivElement | null>(null)
const seriesEl = ref<HTMLDivElement | null>(null)
const dispChart = shallowRef<echarts.ECharts | null>(null)
const seriesChart = shallowRef<echarts.ECharts | null>(null)

function renderCharts() {
  const s = stats.value
  const t = chartTheme.value
  // Dispositions are policy outcomes — reuse the app's status colors.
  const dispositionColor: Record<string, string> = {
    none: t.series.success,
    quarantine: t.series.warning,
    reject: t.series.error,
  }
  if (dispChart.value) {
    dispChart.value.setOption(
      {
        tooltip: {
          trigger: 'item',
          backgroundColor: t.tooltipBg,
          borderColor: t.tooltipBorder,
          textStyle: { color: t.tooltipText },
        },
        legend: { bottom: 0, textStyle: { color: t.legendText } },
        series: [
          {
            type: 'pie',
            radius: ['48%', '72%'],
            itemStyle: { borderColor: t.tooltipBg, borderWidth: 2 },
            label: { show: false },
            data: (s?.dispositions ?? []).map((d) => ({
              name: d.label,
              value: d.count,
              itemStyle: { color: dispositionColor[d.label] ?? t.series.info },
            })),
          },
        ],
      },
      true,
    )
  }
  if (seriesChart.value) {
    const days = s?.series ?? []
    seriesChart.value.setOption(
      {
        tooltip: {
          trigger: 'axis',
          backgroundColor: t.tooltipBg,
          borderColor: t.tooltipBorder,
          textStyle: { color: t.tooltipText },
        },
        legend: { top: 0, textStyle: { color: t.legendText } },
        grid: { left: 48, right: 16, top: 36, bottom: 28 },
        xAxis: {
          type: 'category',
          data: days.map((d) => d.date),
          axisLabel: { color: t.axisLabel },
          axisLine: { lineStyle: { color: t.axisLine } },
        },
        yAxis: {
          type: 'value',
          min: 0,
          axisLabel: { color: t.axisLabel },
          splitLine: { lineStyle: { color: t.splitLine } },
        },
        series: [
          {
            name: 'Messages',
            type: 'bar',
            color: t.series.info,
            barMaxWidth: 28,
            itemStyle: { borderRadius: [3, 3, 0, 0] },
            data: days.map((d) => d.messages),
          },
          {
            name: 'DMARC pass',
            type: 'line',
            smooth: true,
            showSymbol: false,
            lineStyle: { width: 2 },
            color: t.series.success,
            data: days.map((d) => d.pass),
          },
        ],
      },
      true,
    )
  }
}

async function load() {
  loading.value = true
  try {
    stats.value = await dmarcService.stats({
      domain: domain.value || undefined,
      reporter: reporter.value || undefined,
      from: timeWindowMs.value > 0
        ? new Date(Date.now() - timeWindowMs.value).toISOString()
        : undefined,
    })
  } catch (err) {
    stats.value = null
    if (!(err instanceof ApiError && err.notImplemented)) {
      toast({
        title: 'Failed to load DMARC stats',
        description: err instanceof Error ? err.message : 'Unexpected error.',
        variant: 'destructive',
      })
    }
  } finally {
    loading.value = false
  }
}

// ---- Received reports (the raw aggregate reports behind the stats) ----

const {
  items: reports,
  loading: reportsLoading,
  notImplemented: reportsNotImplemented,
  pageSize: reportsPageSize,
  pageNumber: reportsPageNumber,
  hasPrev: reportsHasPrev,
  hasNext: reportsHasNext,
  reload: reloadReports,
  nextPage: reportsNextPage,
  prevPage: reportsPrevPage,
  setPageSize: setReportsPageSize,
} = usePagedList<DmarcReport>({
  loader: (page) => dmarcService.listReports(domain.value || undefined, page),
  pageSize: 25,
})

function policyVariant(p: string) {
  switch ((p || '').toLowerCase()) {
    case 'reject':
      return 'destructive' as const
    case 'quarantine':
      return 'warning' as const
    case 'none':
      return 'success' as const
    default:
      return 'secondary' as const
  }
}

// Report windows are usually whole days — show the compact date part only.
function shortDate(iso: string): string {
  const t = Date.parse(iso)
  return Number.isNaN(t) ? iso : new Date(t).toISOString().slice(0, 10)
}

const onResize = () => {
  dispChart.value?.resize()
  seriesChart.value?.resize()
}

onMounted(async () => {
  if (dispEl.value) dispChart.value = echarts.init(dispEl.value)
  if (seriesEl.value) seriesChart.value = echarts.init(seriesEl.value)
  window.addEventListener('resize', onResize)
  try {
    domains.value = (await dmarcService.domains()).domains ?? []
  } catch {
    domains.value = []
  }
  await load()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  dispChart.value?.dispose()
  seriesChart.value?.dispose()
})

watch([domain, reporter, timeWindowMs], load)
watch(domain, () => reloadReports())
// Re-render whenever data lands, and re-skin when the theme flips.
watch([stats, loading, chartTheme], () => {
  if (stats.value) renderCharts()
})
</script>

<template>
  <div>
    <PageHeader
      title="DMARC Reports"
      description="Aggregated statistics from inbound DMARC aggregate reports. Set the report address in Global Settings and advertise it as rua= in your domains' DMARC records."
    />

    <Card class="mb-4">
      <CardContent class="pa-4">
        <v-row dense>
          <v-col cols="12" sm="6" md="4">
            <v-select
              v-model="domain"
              :items="domainItems"
              data-testid="dmarc-domain-filter"
              label="Domain"
              prepend-inner-icon="mdi-web"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <v-select
              v-model="reporter"
              :items="reporterItems"
              data-testid="dmarc-reporter-filter"
              label="Reporter"
              prepend-inner-icon="mdi-office-building-outline"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <v-select
              v-model="timeWindowMs"
              :items="TIME_ITEMS"
              data-testid="dmarc-window"
              label="Time range"
              prepend-inner-icon="mdi-clock-outline"
              variant="outlined"
              density="compact"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="2" class="d-flex align-center">
            <v-btn
              variant="outlined"
              color="secondary"
              block
              :disabled="!hasActiveFilters"
              data-testid="reset-filters"
              @click="resetFilters"
            >
              Reset
            </v-btn>
          </v-col>
        </v-row>
      </CardContent>
    </Card>

    <v-row dense class="mb-2">
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="Messages"
          :value="total.toLocaleString()"
          caption="Covered by aggregate reports"
          icon="mdi-email-multiple-outline"
          color="primary"
        />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="DMARC Pass"
          :value="`${dmarcRate}%`"
          :caption="`${(stats?.dmarcPass ?? 0).toLocaleString()} aligned`"
          icon="mdi-shield-check-outline"
          :color="total === 0 ? 'secondary' : dmarcRate >= 95 ? 'success' : dmarcRate >= 85 ? 'warning' : 'error'"
          :value-class="passClass(dmarcRate)"
        />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="SPF Pass"
          :value="`${spfRate}%`"
          :caption="`${(stats?.spfPass ?? 0).toLocaleString()} passed`"
          icon="mdi-ip-network-outline"
          :color="total === 0 ? 'secondary' : spfRate >= 95 ? 'success' : spfRate >= 85 ? 'warning' : 'error'"
          :value-class="passClass(spfRate)"
        />
      </v-col>
      <v-col cols="12" sm="6" lg="3">
        <StatTile
          label="DKIM Pass"
          :value="`${dkimRate}%`"
          :caption="`${(stats?.dkimPass ?? 0).toLocaleString()} passed`"
          icon="mdi-key-outline"
          :color="total === 0 ? 'secondary' : dkimRate >= 95 ? 'success' : dkimRate >= 85 ? 'warning' : 'error'"
          :value-class="passClass(dkimRate)"
        />
      </v-col>
    </v-row>

    <v-row dense class="mb-2">
      <v-col cols="12" lg="4">
        <Card class="h-100">
          <CardHeader class="pb-2">
            <CardTitle>Disposition</CardTitle>
            <p class="text-caption text-medium-emphasis mb-0">Receiver policy applied</p>
          </CardHeader>
          <CardContent>
            <div class="position-relative w-100" style="height: 224px">
              <div ref="dispEl" class="h-100 w-100" />
              <div
                v-if="loading || total === 0"
                class="position-absolute top-0 left-0 right-0 bottom-0 d-flex align-center justify-center text-body-2 text-medium-emphasis chart-overlay"
              >
                {{ loading ? 'Loading…' : 'No data' }}
              </div>
            </div>
          </CardContent>
        </Card>
      </v-col>
      <v-col cols="12" lg="8">
        <Card class="h-100">
          <CardHeader class="pb-2">
            <CardTitle>Volume &amp; DMARC Pass</CardTitle>
            <p class="text-caption text-medium-emphasis mb-0">Messages per day</p>
          </CardHeader>
          <CardContent>
            <div class="position-relative w-100" style="height: 224px">
              <div ref="seriesEl" class="h-100 w-100" />
              <div
                v-if="loading || total === 0"
                class="position-absolute top-0 left-0 right-0 bottom-0 d-flex align-center justify-center text-body-2 text-medium-emphasis chart-overlay"
              >
                {{ loading ? 'Loading…' : 'No data' }}
              </div>
            </div>
          </CardContent>
        </Card>
      </v-col>
    </v-row>

    <v-row dense class="mb-2">
      <v-col cols="12" lg="4">
      <Card class="h-100">
        <CardHeader class="pb-2"><CardTitle>Top Source IPs</CardTitle></CardHeader>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Source IP</TableHead>
                <TableHead class="text-right">Messages</TableHead>
                <TableHead class="text-right">Pass</TableHead>
                <TableHead class="text-right">Fail</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableEmpty v-if="!stats?.topSources?.length" :colspan="4" message="No data." />
              <TableRow v-for="s in stats?.topSources ?? []" :key="s.ip">
                <TableCell class="font-mono text-caption">{{ s.ip }}</TableCell>
                <TableCell class="text-right tabular-nums">{{ s.total.toLocaleString() }}</TableCell>
                <TableCell class="text-right tabular-nums text-success">{{ s.pass.toLocaleString() }}</TableCell>
                <TableCell class="text-right tabular-nums" :class="s.fail ? 'text-error' : ''">
                  {{ s.fail.toLocaleString() }}
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      </v-col>
      <v-col cols="12" lg="4">
      <Card class="h-100">
        <CardHeader class="pb-2"><CardTitle>By Domain</CardTitle></CardHeader>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead class="text-right">Messages</TableHead>
                <TableHead class="text-right">Pass rate</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableEmpty v-if="!stats?.domains?.length" :colspan="3" message="No data." />
              <TableRow v-for="d in stats?.domains ?? []" :key="d.domain">
                <TableCell class="font-weight-medium">{{ d.domain }}</TableCell>
                <TableCell class="text-right tabular-nums">{{ d.messages.toLocaleString() }}</TableCell>
                <TableCell class="text-right">
                  <Badge :variant="pct(d.pass, d.messages) >= 95 ? 'success' : 'warning'">
                    {{ pct(d.pass, d.messages) }}%
                  </Badge>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      </v-col>
      <v-col cols="12" lg="4">
      <Card class="h-100">
        <CardHeader class="pb-2">
          <div class="d-flex align-center justify-space-between ga-2">
            <CardTitle>By Reporter</CardTitle>
            <span class="text-caption text-medium-emphasis">Click to drill down</span>
          </div>
        </CardHeader>
        <CardContent class="pa-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Reporter</TableHead>
                <TableHead class="text-right">Messages</TableHead>
                <TableHead class="text-right">Pass rate</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableEmpty v-if="!stats?.reporters?.length" :colspan="3" message="No data." />
              <TableRow
                v-for="r in stats?.reporters ?? []"
                :key="r.reporter"
                :class="reporter === r.reporter ? 'row-clickable row-selected' : 'row-clickable'"
                :title="reporter === r.reporter ? 'Clear reporter filter' : `Drill down to ${r.reporter}`"
                @click="selectReporter(r.reporter)"
              >
                <TableCell class="font-weight-medium text-truncate" style="max-width: 160px">
                  {{ r.reporter }}
                </TableCell>
                <TableCell class="text-right tabular-nums">{{ r.messages.toLocaleString() }}</TableCell>
                <TableCell class="text-right">
                  <Badge :variant="pct(r.pass, r.messages) >= 95 ? 'success' : 'warning'">
                    {{ pct(r.pass, r.messages) }}%
                  </Badge>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      </v-col>
    </v-row>

    <!-- The raw aggregate reports behind the numbers: one row per received
         report, so an odd stat can be traced back to who reported it. -->
    <Card v-if="!reportsNotImplemented">
      <CardHeader class="pb-2">
        <CardTitle>Received Reports</CardTitle>
        <p class="text-caption text-medium-emphasis mb-0">
          Raw aggregate reports<template v-if="domain"> for {{ domain }}</template>
        </p>
      </CardHeader>
      <v-progress-linear :active="reportsLoading" indeterminate color="primary" height="2" />
      <CardContent class="pa-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Received</TableHead>
              <TableHead>Reporter</TableHead>
              <TableHead>Domain</TableHead>
              <TableHead>Report period</TableHead>
              <TableHead>Policy</TableHead>
              <TableHead>Report ID</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableEmpty v-if="reports.length === 0" :colspan="6" message="No reports received yet." />
            <TableRow v-for="r in reports" :key="`${r.orgName}|${r.reportId}`">
              <TableCell class="text-no-wrap text-medium-emphasis">{{ formatDateTime(r.receivedAt) }}</TableCell>
              <TableCell class="font-weight-medium">{{ r.orgName }}</TableCell>
              <TableCell>{{ r.domain }}</TableCell>
              <TableCell class="text-no-wrap tabular-nums text-medium-emphasis">
                {{ shortDate(r.dateBegin) }} → {{ shortDate(r.dateEnd) }}
              </TableCell>
              <TableCell>
                <Badge :variant="policyVariant(r.policyP)">
                  p={{ r.policyP || '—' }}<template v-if="r.policyPct && r.policyPct !== 100"> · {{ r.policyPct }}%</template>
                </Badge>
              </TableCell>
              <TableCell style="max-width: 220px">
                <span class="d-block text-truncate font-mono text-caption text-medium-emphasis" :title="r.reportId">
                  {{ r.reportId }}
                </span>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
    <PaginationControls
      v-if="!reportsNotImplemented && (reports.length > 0 || reportsHasPrev)"
      :page-number="reportsPageNumber"
      :has-prev="reportsHasPrev"
      :has-next="reportsHasNext"
      :loading="reportsLoading"
      :page-size="reportsPageSize"
      @prev="reportsPrevPage"
      @next="reportsNextPage"
      @page-size-change="setReportsPageSize"
    />

    <p v-if="!loading && total === 0" class="mt-4 text-body-2 text-medium-emphasis">
      No DMARC reports yet. Configure the report address in Global Settings and advertise it as
      <code>rua=</code> in your DMARC DNS records.
    </p>
  </div>
</template>

<style scoped>
.row-clickable {
  cursor: pointer;
}
.row-selected {
  background: rgba(var(--v-theme-primary), 0.08);
}
/* Cover the chart canvas so a stale render doesn't show through the state text. */
.chart-overlay {
  background: rgb(var(--v-theme-surface));
}
</style>
