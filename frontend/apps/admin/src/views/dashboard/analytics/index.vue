<script lang="ts" setup>
import type { DashboardClassRow, DashboardSummary } from '#/api/kumo';

import { computed, onMounted, onUnmounted, ref, watch } from 'vue';

import {
  EchartsUI,
  type EchartsUIType,
  useEcharts,
} from '@vben/plugins/echarts';

import {
  Alert,
  Card,
  Col,
  Radio,
  RadioGroup,
  Row,
  Statistic,
  Table,
  Tag,
} from 'ant-design-vue';
import dayjs from 'dayjs';

import { dashboardApi } from '#/api/kumo';

defineOptions({ name: 'Analytics' });

// ─────────────────────────────────────────────────────────────────────────────
// State
// ─────────────────────────────────────────────────────────────────────────────

const summary = ref<DashboardSummary | null>(null);
const byClass = ref<DashboardClassRow[]>([]);
const loadError = ref<string>('');
const metricsConfigured = ref(true);

// Range selector for the trend chart. Mapped to (range, step) pairs
// the backend will accept; the server further clamps these.
type RangeKey = '1h' | '6h' | '24h' | '7d';
const rangeKey = ref<RangeKey>('1h');
const RANGE_STEP: Record<RangeKey, { range: string; step: string }> = {
  '1h': { range: '1h', step: '30s' },
  '6h': { range: '6h', step: '2m' },
  '24h': { range: '24h', step: '5m' },
  '7d': { range: '7d', step: '30m' },
};

// Chart ref + the @vben/plugins/echarts hook.
const chartRef = ref<EchartsUIType>();
const { renderEcharts } = useEcharts(chartRef);

// Stable colour mapping per event type so the legend stays intuitive
// across refreshes — green for delivered, red for bounce, etc.
const EVENT_COLOR: Record<string, string> = {
  Reception: '#1890ff',
  Delivery: '#52c41a',
  Bounce: '#f5222d',
  TransientFailure: '#fa8c16',
  Feedback: '#722ed1',
};
const colorFor = (t: string) => EVENT_COLOR[t] ?? '#8c8c8c';

// ─────────────────────────────────────────────────────────────────────────────
// Cards derived from the summary
// ─────────────────────────────────────────────────────────────────────────────

// Numbers are rounded for display — Prometheus's increase() returns
// floats due to extrapolation, which is awkward to read on a card.
const cards = computed(() => {
  const s = summary.value;
  if (!s) return [];
  const reception = Math.round(s.events_24h?.Reception ?? 0);
  const delivery = Math.round(s.events_24h?.Delivery ?? 0);
  const bounce = Math.round(s.events_24h?.Bounce ?? 0);
  const transient = Math.round(s.events_24h?.TransientFailure ?? 0);

  return [
    {
      title: 'Messages received (24h)',
      value: reception,
      suffix: '',
      tone: 'neutral',
      footnote: '',
    },
    {
      title: 'Delivery rate (24h)',
      value: (s.delivery_rate_24h * 100).toFixed(1),
      suffix: '%',
      tone:
        s.delivery_rate_24h >= 0.9
          ? 'good'
          : s.delivery_rate_24h >= 0.7
            ? 'warn'
            : 'bad',
      footnote: `${delivery.toLocaleString()} delivered / ${reception.toLocaleString()} received`,
    },
    {
      title: 'Bounce rate (24h)',
      value: (s.bounce_rate_24h * 100).toFixed(2),
      suffix: '%',
      tone:
        s.bounce_rate_24h <= 0.05
          ? 'good'
          : s.bounce_rate_24h <= 0.1
            ? 'warn'
            : 'bad',
      footnote: `${bounce.toLocaleString()} bounced · ${transient.toLocaleString()} retried`,
    },
    {
      title: 'Stream backlog',
      value: Math.round(s.stream_pending),
      suffix: '',
      tone:
        s.stream_pending === 0
          ? 'good'
          : s.stream_pending < 1000
            ? 'warn'
            : 'bad',
      footnote: 'XPENDING on kumo.events',
    },
    {
      title: 'Suppression entries',
      value: Math.round(
        (s.suppression_entries?.address ?? 0) +
          (s.suppression_entries?.domain ?? 0),
      ),
      suffix: '',
      tone: 'neutral',
      footnote: `${s.suppression_entries?.address ?? 0} addr · ${s.suppression_entries?.domain ?? 0} dom`,
    },
    {
      title: 'Policy applies (24h)',
      value: Math.round(
        (s.policy_applies_24h?.ok ?? 0) +
          (s.policy_applies_24h?.error ?? 0),
      ),
      suffix: '',
      tone: (s.policy_applies_24h?.error ?? 0) > 0 ? 'bad' : 'good',
      footnote: `${Math.round(s.policy_applies_24h?.ok ?? 0)} ok · ${Math.round(s.policy_applies_24h?.error ?? 0)} error`,
    },
  ];
});

const generatedAt = computed(() =>
  summary.value
    ? `as of ${dayjs(summary.value.generated_at).format('HH:mm:ss')}`
    : '',
);

// ─────────────────────────────────────────────────────────────────────────────
// Loaders
// ─────────────────────────────────────────────────────────────────────────────

async function loadSummary() {
  try {
    summary.value = await dashboardApi.summary();
    byClass.value = (await dashboardApi.byClass()).classes;
    metricsConfigured.value = true;
    loadError.value = '';
  } catch (e: any) {
    handleApiError(e);
  }
}

async function loadChart() {
  try {
    const r = await dashboardApi.eventRates(
      RANGE_STEP[rangeKey.value].range,
      RANGE_STEP[rangeKey.value].step,
    );
    renderEcharts({
      grid: { left: 50, right: 16, top: 30, bottom: 40 },
      legend: { bottom: 0, type: 'scroll' },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
        valueFormatter: (v: any) =>
          typeof v === 'number' ? `${v.toFixed(2)} /s` : String(v),
      },
      xAxis: { type: 'time', boundaryGap: false } as any,
      yAxis: {
        type: 'value',
        name: 'events / sec',
        nameLocation: 'middle',
        nameGap: 35,
      },
      series: r.series.map((s) => ({
        name: s.event_type,
        type: 'line',
        smooth: true,
        showSymbol: false,
        sampling: 'lttb',
        lineStyle: { width: 2 },
        itemStyle: { color: colorFor(s.event_type) },
        areaStyle: { color: colorFor(s.event_type), opacity: 0.08 },
        data: s.points.map((p) => [p.at, p.value]),
      })),
    });
  } catch (e: any) {
    handleApiError(e);
  }
}

function handleApiError(e: any) {
  const code = e?.response?.data?.code;
  if (code === 'METRICS_NOT_CONFIGURED') {
    metricsConfigured.value = false;
    loadError.value = '';
    return;
  }
  loadError.value =
    e?.response?.data?.message || e?.message || 'failed to load dashboard';
}

async function loadAll() {
  await loadSummary();
  if (metricsConfigured.value) {
    await loadChart();
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Lifecycle — auto-refresh every 15s. Matches the Prometheus scrape
// interval; refreshing faster than that just spends bytes on the same
// numbers.
// ─────────────────────────────────────────────────────────────────────────────

let timer: null | ReturnType<typeof setInterval> = null;

onMounted(() => {
  loadAll();
  timer = setInterval(loadAll, 15_000);
});

onUnmounted(() => {
  if (timer) clearInterval(timer);
});

watch(rangeKey, () => loadChart());

// ─────────────────────────────────────────────────────────────────────────────
// Class breakdown table
// ─────────────────────────────────────────────────────────────────────────────

const classColumns = [
  {
    title: 'Mail class',
    dataIndex: 'mail_class',
    key: 'mail_class',
    customRender: ({ text }: { text: string }) =>
      text || '— (unclassified)',
  },
  {
    title: 'Events (24h)',
    dataIndex: 'events_24h',
    key: 'events_24h',
    align: 'right' as const,
    customRender: ({ text }: { text: number }) =>
      Math.round(text).toLocaleString(),
  },
  {
    title: 'Delivery rate',
    dataIndex: 'delivery_rate',
    key: 'delivery_rate',
    align: 'right' as const,
  },
];
</script>

<template>
  <div class="p-5">
    <Alert
      v-if="!metricsConfigured"
      type="warning"
      message="Metrics backend not configured"
      description="Set IRIS_PROMETHEUS_URL on the admin-service container so the dashboard can query Prometheus. Until then the cards stay blank — the rest of the operator UI is unaffected."
      show-icon
      class="mb-4"
    />
    <Alert
      v-else-if="loadError"
      type="error"
      message="Failed to refresh dashboard"
      :description="loadError"
      show-icon
      class="mb-4"
    />

    <!-- Summary cards -->
    <Row :gutter="[16, 16]">
      <Col v-for="c in cards" :key="c.title" :xs="24" :sm="12" :md="8" :xl="4">
        <Card :body-style="{ padding: '16px' }" :bordered="false">
          <Statistic
            :title="c.title"
            :value="c.value"
            :suffix="c.suffix"
            :value-style="{
              color:
                c.tone === 'good'
                  ? '#52c41a'
                  : c.tone === 'bad'
                    ? '#f5222d'
                    : c.tone === 'warn'
                      ? '#fa8c16'
                      : undefined,
            }"
          />
          <div v-if="c.footnote" class="footnote">{{ c.footnote }}</div>
        </Card>
      </Col>
    </Row>

    <!-- Trend chart -->
    <Card class="mt-4" :body-style="{ padding: '16px' }">
      <div class="chart-header">
        <div>
          <strong>Event rate</strong>
          <span class="dim">{{ generatedAt }}</span>
        </div>
        <RadioGroup v-model:value="rangeKey" size="small">
          <Radio value="1h">1h</Radio>
          <Radio value="6h">6h</Radio>
          <Radio value="24h">24h</Radio>
          <Radio value="7d">7d</Radio>
        </RadioGroup>
      </div>
      <EchartsUI ref="chartRef" style="height: 320px" />
    </Card>

    <!-- Mail-class breakdown -->
    <Card class="mt-4" :body-style="{ padding: '16px' }">
      <div class="chart-header">
        <strong>Volume by mail class (24h)</strong>
      </div>
      <Table
        :columns="classColumns"
        :data-source="byClass"
        :pagination="false"
        size="small"
        row-key="mail_class"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'delivery_rate'">
            <Tag
              :color="
                record.delivery_rate >= 0.9
                  ? 'green'
                  : record.delivery_rate >= 0.7
                    ? 'orange'
                    : 'red'
              "
            >
              {{ (record.delivery_rate * 100).toFixed(1) }}%
            </Tag>
          </template>
        </template>
      </Table>
    </Card>
  </div>
</template>

<style scoped>
.footnote {
  margin-top: 4px;
  font-size: 12px;
  color: var(--ant-color-text-tertiary);
}
.chart-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.dim {
  margin-left: 12px;
  font-size: 12px;
  color: var(--ant-color-text-tertiary);
}
</style>
