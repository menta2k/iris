<script lang="ts" setup>
import type { DsnEntry } from '#/api/kumo';

import type { Dayjs } from 'dayjs';

import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { Page } from '@vben/common-ui';
import { useIntervalFn } from '@vueuse/core';

import {
  Button,
  Card,
  DatePicker,
  Form,
  FormItem,
  Input,
  Modal,
  Select,
  SelectOption,
  Space,
  Switch,
  Table,
  Tag,
} from 'ant-design-vue';
import dayjs from 'dayjs';

import { dsnsApi } from '#/api/kumo';

defineOptions({ name: 'Dsns' });

const route = useRoute();
const router = useRouter();

const items = ref<DsnEntry[]>([]);
const loading = ref(false);
const autoRefresh = ref(false);

// Search form. Empty strings = "don't constrain"; the URL query state is
// the source of truth so a refresh / shared link reproduces the same view.
const filters = reactive({
  category: queryString(route.query.category),
  status_class: queryString(route.query.status_class),
  recipient: queryString(route.query.recipient),
  mail_class: queryString(route.query.mail_class),
  message_id: queryString(route.query.message_id),
  range: parseRange(route.query.since, route.query.until),
});

function queryString(v: any): string {
  return typeof v === 'string' ? v : '';
}

function parseRange(s: any, u: any): [Dayjs | null, Dayjs | null] {
  const since = typeof s === 'string' && s ? dayjs(s) : null;
  const until = typeof u === 'string' && u ? dayjs(u) : null;
  return [since, until];
}

const columns = [
  { title: 'Time', dataIndex: 'received_at', key: 'received_at', width: 180 },
  {
    title: 'Recipient',
    dataIndex: 'final_recipient',
    key: 'final_recipient',
    width: 240,
  },
  {
    title: 'Category',
    dataIndex: 'category',
    key: 'category',
    width: 160,
  },
  {
    title: 'Status',
    dataIndex: 'status',
    key: 'status',
    width: 90,
    align: 'center' as const,
  },
  {
    title: 'Class',
    dataIndex: 'mail_class',
    key: 'mail_class',
    width: 120,
  },
  {
    title: 'Diagnostic',
    dataIndex: 'diagnostic_code',
    key: 'diagnostic_code',
    ellipsis: true,
  },
  {
    title: 'Message ID',
    dataIndex: 'message_id_ref',
    key: 'message_id_ref',
    width: 220,
    ellipsis: true,
  },
  { title: '', dataIndex: '_actions', key: '_actions', width: 80 },
];

// CATEGORY_OPTIONS mirrors pkg/bounceclass.Category — keep in sync.
const CATEGORY_OPTIONS = [
  'unknown_user',
  'mailbox_full',
  'mailbox_disabled',
  'policy_block',
  'reputation_block',
  'auth_failed',
  'content_rejected',
  'routing_failed',
  'relay_denied',
  'transient_net',
  'transient_spam',
  'transient_other',
  'hard_other',
  'unknown',
];

// status_class is a coarse "hard / soft" split on top of the X.Y.Z code.
// Two buttons keep the common operator workflow (hard vs all transient)
// one click away.
const STATUS_CLASS_OPTIONS = [
  { label: 'Hard (5.x.x)', value: '5' },
  { label: 'Soft (4.x.x)', value: '4' },
];

const activeFilterCount = computed(() => {
  let n = 0;
  if (filters.category) n++;
  if (filters.status_class) n++;
  if (filters.recipient) n++;
  if (filters.mail_class) n++;
  if (filters.message_id) n++;
  if (filters.range[0] || filters.range[1]) n++;
  return n;
});

// Per-category colour. Hard buckets get red shades, soft / unknown get
// muted ones — operators eyeball this page looking for spikes, so colour
// is the primary signal.
const HARD_RED = new Set([
  'unknown_user',
  'mailbox_disabled',
  'policy_block',
  'reputation_block',
  'auth_failed',
  'content_rejected',
  'hard_other',
]);
function categoryColor(c?: string): string {
  if (!c) return 'default';
  if (HARD_RED.has(c)) return 'red';
  if (c === 'mailbox_full') return 'volcano';
  if (c === 'routing_failed' || c === 'relay_denied') return 'orange';
  if (c.startsWith('transient_')) return 'gold';
  return 'default';
}

function statusColor(s?: string): string {
  if (!s) return 'default';
  if (s.startsWith('5.')) return 'red';
  if (s.startsWith('4.')) return 'gold';
  return 'default';
}

async function load() {
  loading.value = true;
  try {
    const params: Record<string, string | number | undefined> = { limit: 500 };
    if (filters.category) params.category = filters.category;
    if (filters.status_class) params.status_class = filters.status_class;
    if (filters.recipient) params.recipient = filters.recipient;
    if (filters.mail_class) params.mail_class = filters.mail_class;
    if (filters.message_id) params.message_id = filters.message_id;
    if (filters.range[0]) params.since = filters.range[0].toISOString();
    if (filters.range[1]) params.until = filters.range[1].toISOString();
    const r = await dsnsApi.list(params as any);
    items.value = r.items ?? [];
  } finally {
    loading.value = false;
  }
}

function applyFilters() {
  // Reflect filters into the URL so the search state survives reloads
  // and is shareable (same pattern as the Logs page).
  const query: Record<string, string> = {};
  if (filters.category) query.category = filters.category;
  if (filters.status_class) query.status_class = filters.status_class;
  if (filters.recipient) query.recipient = filters.recipient;
  if (filters.mail_class) query.mail_class = filters.mail_class;
  if (filters.message_id) query.message_id = filters.message_id;
  if (filters.range[0]) query.since = filters.range[0].toISOString();
  if (filters.range[1]) query.until = filters.range[1].toISOString();
  router.replace({ query });
  load();
}

function clearFilters() {
  filters.category = '';
  filters.status_class = '';
  filters.recipient = '';
  filters.mail_class = '';
  filters.message_id = '';
  filters.range = [null, null];
  applyFilters();
}

// Drill-through: clicking a message_id pivots to the Logs page filtered
// to the same id. Operators want to see the full timeline of the
// failed message — Reception, every TransientFailure, and the eventual
// Bounce — without manually re-typing the id.
function gotoLogs(id?: string) {
  if (!id) return;
  router.push({ path: '/observability/logs', query: { message_id: id } });
}

const detail = ref<{ open: boolean; row: DsnEntry | null }>({
  open: false,
  row: null,
});
function openDetail(row: DsnEntry) {
  detail.value = { open: true, row };
}

// Pretty-print the embedded headers from extra_json. We swallow parse
// errors silently — the column is best-effort and a malformed row
// shouldn't block the modal.
const detailMeta = computed<Record<string, unknown> | null>(() => {
  const j = detail.value.row?.extra_json;
  if (!j) return null;
  try {
    return JSON.parse(j);
  } catch {
    return null;
  }
});

// 30-second auto-refresh — bounces accumulate slower than log events,
// so the more aggressive 10s interval used on the Logs page would just
// burn API calls without changing the visible state.
const { pause, resume } = useIntervalFn(load, 30_000, { immediate: false });

function toggleAuto(on: boolean | string | number) {
  const enabled = Boolean(on);
  autoRefresh.value = enabled;
  if (enabled) resume();
  else pause();
}

watch(
  () => route.query,
  () => {
    filters.category = queryString(route.query.category);
    filters.status_class = queryString(route.query.status_class);
    filters.recipient = queryString(route.query.recipient);
    filters.mail_class = queryString(route.query.mail_class);
    filters.message_id = queryString(route.query.message_id);
    filters.range = parseRange(route.query.since, route.query.until);
  },
);

onMounted(load);
onUnmounted(pause);
</script>

<template>
  <Page
    title="Bounces"
    description="Async DSN events parsed from inbound bounce mail. Hard bounces auto-add to the suppression list."
  >
    <Card :body-style="{ padding: '16px' }">
      <Form layout="inline" class="mb-3" @submit.prevent="applyFilters">
        <FormItem label="Category">
          <Select
            v-model:value="filters.category"
            allow-clear
            placeholder="any"
            style="width: 200px"
            @change="applyFilters"
          >
            <SelectOption v-for="c in CATEGORY_OPTIONS" :key="c" :value="c">
              {{ c }}
            </SelectOption>
          </Select>
        </FormItem>

        <FormItem label="Severity">
          <Select
            v-model:value="filters.status_class"
            allow-clear
            placeholder="any"
            style="width: 160px"
            @change="applyFilters"
          >
            <SelectOption
              v-for="o in STATUS_CLASS_OPTIONS"
              :key="o.value"
              :value="o.value"
            >
              {{ o.label }}
            </SelectOption>
          </Select>
        </FormItem>

        <FormItem label="Recipient">
          <Input
            v-model:value="filters.recipient"
            allow-clear
            placeholder="contains…"
            style="width: 200px"
            @press-enter="applyFilters"
          />
        </FormItem>

        <FormItem label="Class">
          <Input
            v-model:value="filters.mail_class"
            allow-clear
            placeholder="contains…"
            style="width: 160px"
            @press-enter="applyFilters"
          />
        </FormItem>

        <FormItem label="Message ID">
          <Input
            v-model:value="filters.message_id"
            allow-clear
            placeholder="exact id…"
            style="width: 240px"
            @press-enter="applyFilters"
          />
        </FormItem>

        <FormItem label="Time">
          <DatePicker.RangePicker
            :value="filters.range as any"
            show-time
            :allow-empty="[true, true]"
            format="YYYY-MM-DD HH:mm"
            style="width: 320px"
            @change="(val: any) => { filters.range = (val ?? [null, null]) as [Dayjs | null, Dayjs | null]; applyFilters(); }"
          />
        </FormItem>

        <FormItem>
          <Space>
            <Button type="primary" :loading="loading" html-type="submit" @click="applyFilters">
              Search
            </Button>
            <Button :disabled="activeFilterCount === 0" @click="clearFilters">
              Clear
            </Button>
            <span style="color: var(--ant-color-text-tertiary)">
              <span>Auto&nbsp;</span>
              <Switch :checked="autoRefresh" size="small" @change="toggleAuto" />
            </span>
          </Space>
        </FormItem>
      </Form>

      <Table
        :columns="columns"
        :data-source="items"
        :loading="loading"
        :pagination="{ pageSize: 50, showSizeChanger: true }"
        row-key="id"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'category'">
            <Tag v-if="record.category" :color="categoryColor(record.category)">
              {{ record.category }}
            </Tag>
            <span v-else style="color: var(--ant-color-text-quaternary)">—</span>
          </template>
          <template v-else-if="column.key === 'status'">
            <Tag v-if="record.status" :color="statusColor(record.status)">
              {{ record.status }}
            </Tag>
            <span v-else style="color: var(--ant-color-text-quaternary)">—</span>
          </template>
          <template v-else-if="column.key === 'mail_class'">
            <Tag v-if="record.mail_class" color="purple">
              {{ record.mail_class }}
            </Tag>
            <span v-else style="color: var(--ant-color-text-quaternary)">—</span>
          </template>
          <template v-else-if="column.key === 'message_id_ref'">
            <a
              v-if="record.message_id_ref"
              class="msgid-link"
              title="Open in Log Stream"
              @click="gotoLogs(record.message_id_ref)"
            >
              {{ record.message_id_ref }}
            </a>
            <span v-else style="color: var(--ant-color-text-quaternary)">—</span>
          </template>
          <template v-else-if="column.key === '_actions'">
            <Button size="small" @click="openDetail(record as DsnEntry)">Details</Button>
          </template>
        </template>
      </Table>
    </Card>

    <Modal
      v-model:open="detail.open"
      :title="`DSN — ${detail.row?.final_recipient ?? ''}`"
      :footer="null"
      width="780px"
      destroy-on-close
    >
      <div v-if="detail.row" class="dsn-detail">
        <div class="kv">
          <span class="k">Received at</span>
          <span class="v">{{ detail.row.received_at }}</span>
        </div>
        <div class="kv">
          <span class="k">Final recipient</span>
          <span class="v">{{ detail.row.final_recipient || '—' }}</span>
        </div>
        <div class="kv">
          <span class="k">Original recipient</span>
          <span class="v">{{ detail.row.original_recipient || '—' }}</span>
        </div>
        <div class="kv">
          <span class="k">Action</span>
          <span class="v">{{ detail.row.action || '—' }}</span>
        </div>
        <div class="kv">
          <span class="k">Status</span>
          <span class="v">{{ detail.row.status || '—' }}</span>
        </div>
        <div class="kv">
          <span class="k">Category</span>
          <span class="v">{{ detail.row.category || '—' }}</span>
        </div>
        <div class="kv">
          <span class="k">Mail class</span>
          <span class="v">{{ detail.row.mail_class || '—' }}</span>
        </div>
        <div class="kv">
          <span class="k">Tenant</span>
          <span class="v">{{ detail.row.tenant || '—' }}</span>
        </div>
        <div class="kv">
          <span class="k">Remote MTA</span>
          <span class="v">{{ detail.row.remote_mta || '—' }}</span>
        </div>
        <div class="kv">
          <span class="k">Diagnostic</span>
          <span class="v multiline">{{ detail.row.diagnostic_code || '—' }}</span>
        </div>
        <div class="kv">
          <span class="k">Message ID</span>
          <span class="v">
            <a
              v-if="detail.row.message_id_ref"
              class="msgid-link"
              @click="gotoLogs(detail.row.message_id_ref)"
            >
              {{ detail.row.message_id_ref }}
            </a>
            <span v-else>—</span>
          </span>
        </div>
        <div class="kv">
          <span class="k">VERP token</span>
          <span class="v">{{ detail.row.verp_token || '—' }}</span>
        </div>
        <div v-if="detailMeta" class="kv">
          <span class="k">Embedded headers</span>
          <pre class="v meta">{{ JSON.stringify(detailMeta, null, 2) }}</pre>
        </div>
      </div>
    </Modal>
  </Page>
</template>

<style scoped>
.msgid-link {
  font-family: ui-monospace, 'SFMono-Regular', 'Menlo', 'Consolas', monospace;
  font-size: 12px;
  color: var(--ant-color-link);
  cursor: pointer;
}
.msgid-link:hover {
  text-decoration: underline;
}
.dsn-detail {
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
}
.kv {
  display: grid;
  grid-template-columns: 180px 1fr;
  gap: 12px;
  align-items: start;
  font-size: 13px;
}
.kv .k {
  color: var(--ant-color-text-tertiary);
}
.kv .v {
  word-break: break-all;
}
.kv .v.multiline {
  white-space: pre-wrap;
}
.kv .v.meta {
  font-family: ui-monospace, 'SFMono-Regular', 'Menlo', 'Consolas', monospace;
  font-size: 12px;
  background: var(--ant-color-bg-elevated);
  border: 1px solid var(--ant-color-border);
  border-radius: 6px;
  padding: 8px;
  margin: 0;
  max-height: 280px;
  overflow: auto;
}
</style>
