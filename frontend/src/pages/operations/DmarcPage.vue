<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import * as echarts from 'echarts/core'
import { BarChart, LineChart, PieChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import PageHeader from '@/components/common/PageHeader.vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Select } from '@/components/ui/select'
import { useToast } from '@/composables/useToast'
import { dmarcService } from '@/services'
import { ApiError } from '@/services/http'
import type { DmarcStats } from '@/types'

echarts.use([BarChart, LineChart, PieChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const { toast } = useToast()

const domains = ref<string[]>([])
const domain = ref('')
const loading = ref(false)
const stats = ref<DmarcStats | null>(null)

const pct = (n: number, total: number) => (total > 0 ? Math.round((n / total) * 1000) / 10 : 0)
const total = computed(() => stats.value?.totalMessages ?? 0)

const dispEl = ref<HTMLDivElement | null>(null)
const seriesEl = ref<HTMLDivElement | null>(null)
const dispChart = shallowRef<echarts.ECharts | null>(null)
const seriesChart = shallowRef<echarts.ECharts | null>(null)

const DISPOSITION_COLORS: Record<string, string> = {
  none: '#16a34a',
  quarantine: '#d97706',
  reject: '#dc2626',
}

function renderCharts() {
  const s = stats.value
  if (dispChart.value) {
    dispChart.value.setOption(
      {
        tooltip: { trigger: 'item' },
        legend: { bottom: 0, textStyle: { color: '#64748b' } },
        series: [
          {
            type: 'pie',
            radius: ['45%', '70%'],
            data: (s?.dispositions ?? []).map((d) => ({
              name: d.label,
              value: d.count,
              itemStyle: { color: DISPOSITION_COLORS[d.label] },
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
        tooltip: { trigger: 'axis' },
        legend: { top: 0, textStyle: { color: '#64748b' } },
        grid: { left: 48, right: 16, top: 36, bottom: 28 },
        xAxis: { type: 'category', data: days.map((d) => d.date), axisLabel: { color: '#64748b' } },
        yAxis: { type: 'value', min: 0, axisLabel: { color: '#64748b' } },
        series: [
          { name: 'Messages', type: 'bar', color: '#2563eb', data: days.map((d) => d.messages) },
          { name: 'DMARC pass', type: 'line', smooth: true, color: '#16a34a', data: days.map((d) => d.pass) },
        ],
      },
      true,
    )
  }
}

async function load() {
  loading.value = true
  try {
    stats.value = await dmarcService.stats(domain.value || undefined)
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

watch(domain, load)
watch([stats, loading], () => {
  if (stats.value) renderCharts()
})
</script>

<template>
  <div>
    <PageHeader
      title="DMARC Reports"
      description="Aggregated statistics from inbound DMARC aggregate reports. Set the report address in Global Settings and advertise it as rua= in your domains' DMARC records."
    >
      <template #actions>
        <Select v-model="domain" class="w-56" data-testid="dmarc-domain-filter">
          <option value="">All domains</option>
          <option v-for="d in domains" :key="d" :value="d">{{ d }}</option>
        </Select>
      </template>
    </PageHeader>

    <div class="mb-4 grid grid-cols-2 gap-4 md:grid-cols-4">
      <Card>
        <CardHeader class="pb-1"><CardTitle class="text-sm text-muted-foreground">Messages</CardTitle></CardHeader>
        <CardContent class="text-2xl font-semibold tabular-nums">{{ total.toLocaleString() }}</CardContent>
      </Card>
      <Card>
        <CardHeader class="pb-1"><CardTitle class="text-sm text-muted-foreground">DMARC pass</CardTitle></CardHeader>
        <CardContent class="text-2xl font-semibold tabular-nums">
          {{ pct(stats?.dmarcPass ?? 0, total) }}%
        </CardContent>
      </Card>
      <Card>
        <CardHeader class="pb-1"><CardTitle class="text-sm text-muted-foreground">SPF pass</CardTitle></CardHeader>
        <CardContent class="text-2xl font-semibold tabular-nums">{{ pct(stats?.spfPass ?? 0, total) }}%</CardContent>
      </Card>
      <Card>
        <CardHeader class="pb-1"><CardTitle class="text-sm text-muted-foreground">DKIM pass</CardTitle></CardHeader>
        <CardContent class="text-2xl font-semibold tabular-nums">{{ pct(stats?.dkimPass ?? 0, total) }}%</CardContent>
      </Card>
    </div>

    <div class="mb-4 grid gap-4 lg:grid-cols-3">
      <Card>
        <CardHeader><CardTitle class="text-sm text-muted-foreground">Disposition</CardTitle></CardHeader>
        <CardContent><div ref="dispEl" class="h-56 w-full" /></CardContent>
      </Card>
      <Card class="lg:col-span-2">
        <CardHeader><CardTitle class="text-sm text-muted-foreground">Volume & DMARC pass / day</CardTitle></CardHeader>
        <CardContent><div ref="seriesEl" class="h-56 w-full" /></CardContent>
      </Card>
    </div>

    <div class="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader><CardTitle class="text-sm">Top source IPs</CardTitle></CardHeader>
        <CardContent class="p-0">
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
              <TableRow v-for="s in stats?.topSources ?? []" :key="s.ip">
                <TableCell class="font-mono text-xs">{{ s.ip }}</TableCell>
                <TableCell class="text-right tabular-nums">{{ s.total.toLocaleString() }}</TableCell>
                <TableCell class="text-right tabular-nums text-green-600">{{ s.pass.toLocaleString() }}</TableCell>
                <TableCell class="text-right tabular-nums" :class="s.fail ? 'text-destructive' : ''">
                  {{ s.fail.toLocaleString() }}
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle class="text-sm">By domain</CardTitle></CardHeader>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead class="text-right">Messages</TableHead>
                <TableHead class="text-right">Pass rate</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="d in stats?.domains ?? []" :key="d.domain">
                <TableCell class="font-medium">{{ d.domain }}</TableCell>
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
    </div>

    <p v-if="!loading && total === 0" class="mt-4 text-sm text-muted-foreground">
      No DMARC reports yet. Configure the report address in Global Settings and advertise it as
      <code>rua=</code> in your DMARC DNS records.
    </p>
  </div>
</template>
