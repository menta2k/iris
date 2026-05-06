<script lang="ts" setup>
import type { LogEntry } from '#/api/kumo';

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
  Select,
  SelectOption,
  Space,
  Switch,
  Table,
  Tag,
} from 'ant-design-vue';
import dayjs from 'dayjs';

import { logsApi } from '#/api/kumo';

defineOptions({ name: 'Logs' });

const route = useRoute();
const router = useRouter();

const items = ref<LogEntry[]>([]);
const loading = ref(false);
const autoRefresh = ref(false);

// Search form. Empty strings = "don't constrain"; the URL query state is the
// source of truth so a refresh / shared link reproduces the same view.
const filters = reactive({
  event_type: queryString(route.query.event_type),
  sender: queryString(route.query.sender),
  recipient: queryString(route.query.recipient),
  mail_class: queryString(route.query.mail_class),
  // message_id ties the original Reception, every TransientFailure
  // retry, and the eventual Delivery / Bounce together. Click a
  // message_id cell in the table to one-click-filter to the full timeline.
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
  { title: 'Time', dataIndex: 'at', key: 'at', width: 180 },
  { title: 'Event', dataIndex: 'event_type', key: 'event_type', width: 130 },
  { title: 'Sender', dataIndex: 'sender', key: 'sender', width: 220 },
  { title: 'Recipient', dataIndex: 'recipient', key: 'recipient', width: 220 },
  {
    title: 'Class',
    dataIndex: 'mail_class',
    key: 'mail_class',
    width: 130,
  },
  {
    title: 'VMTA',
    dataIndex: 'vmta',
    key: 'vmta',
    width: 160,
    ellipsis: true,
  },
  {
    title: 'Message ID',
    dataIndex: 'message_id',
    key: 'message_id',
    width: 240,
    ellipsis: true,
  },
  {
    title: 'Code',
    dataIndex: 'response_code',
    key: 'response_code',
    width: 80,
    align: 'right' as const,
  },
  {
    title: 'Response',
    dataIndex: 'response_text',
    key: 'response_text',
    ellipsis: true,
  },
];

// kumomta event_type values are PascalCase (Reception, Delivery, …); we
// keep the option list aligned with what the consumer actually inserts.
const EVENT_OPTIONS = [
  'Reception',
  'Delivery',
  'TransientFailure',
  'Bounce',
  'Feedback',
  'AdminBounce',
  'Expired',
];

const activeFilterCount = computed(() => {
  let n = 0;
  if (filters.event_type) n++;
  if (filters.sender) n++;
  if (filters.recipient) n++;
  if (filters.mail_class) n++;
  if (filters.message_id) n++;
  if (filters.range[0] || filters.range[1]) n++;
  return n;
});

async function load() {
  loading.value = true;
  try {
    const params: Record<string, string | number | undefined> = { limit: 500 };
    if (filters.event_type) params.event_type = filters.event_type;
    if (filters.sender) params.sender = filters.sender;
    if (filters.recipient) params.recipient = filters.recipient;
    if (filters.mail_class) params.mail_class = filters.mail_class;
    if (filters.message_id) params.message_id = filters.message_id;
    if (filters.range[0]) params.since = filters.range[0].toISOString();
    if (filters.range[1]) params.until = filters.range[1].toISOString();
    const r = await logsApi.list(params as any);
    items.value = r.items ?? [];
  } finally {
    loading.value = false;
  }
}

function applyFilters() {
  // Reflect filters into the URL so the search state survives reloads and
  // can be linked / bookmarked. Replaces (not pushes) so the back button
  // doesn't accumulate every keystroke as a history entry.
  const query: Record<string, string> = {};
  if (filters.event_type) query.event_type = filters.event_type;
  if (filters.sender) query.sender = filters.sender;
  if (filters.recipient) query.recipient = filters.recipient;
  if (filters.mail_class) query.mail_class = filters.mail_class;
  if (filters.message_id) query.message_id = filters.message_id;
  if (filters.range[0]) query.since = filters.range[0].toISOString();
  if (filters.range[1]) query.until = filters.range[1].toISOString();
  router.replace({ query });
  load();
}

function clearFilters() {
  filters.event_type = '';
  filters.sender = '';
  filters.recipient = '';
  filters.mail_class = '';
  filters.message_id = '';
  filters.range = [null, null];
  applyFilters();
}

// Clicking a message_id cell pins the filter to that id — so the operator
// sees only the events for that single submission (Reception, every
// retry, and the final Delivery/Bounce). When the filter is already on
// the same id, clicking again clears it (toggle behaviour).
function filterToMessage(id: string) {
  if (!id) return;
  filters.message_id = filters.message_id === id ? '' : id;
  // Drop the time range when scoping to a single message — the events
  // for one delivery span the retry window and a stale "today" filter
  // would otherwise hide the historical retries.
  if (filters.message_id) {
    filters.range = [null, null];
  }
  applyFilters();
}

function eventColor(t: string): string {
  switch (t) {
    case 'Delivery': {
      return 'green';
    }
    case 'Reception': {
      return 'blue';
    }
    case 'TransientFailure': {
      return 'orange';
    }
    case 'Bounce':
    case 'AdminBounce':
    case 'Expired': {
      return 'red';
    }
    case 'Feedback': {
      return 'volcano';
    }
    default: {
      return 'default';
    }
  }
}

const { pause, resume } = useIntervalFn(load, 10_000, { immediate: false });

function toggleAuto(on: boolean | string | number) {
  const enabled = Boolean(on);
  autoRefresh.value = enabled;
  if (enabled) resume();
  else pause();
}

// Keep the form in sync if the URL is changed externally (back button, link).
watch(
  () => route.query,
  () => {
    filters.event_type = queryString(route.query.event_type);
    filters.sender = queryString(route.query.sender);
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
  <Page title="Log Stream" description="Live delivery event stream — search by sender, recipient, mail class, or time range">
    <Card :body-style="{ padding: '16px' }">
      <Form layout="inline" class="mb-3" @submit.prevent="applyFilters">
        <FormItem label="Event">
          <Select
            v-model:value="filters.event_type"
            allow-clear
            placeholder="any"
            style="width: 180px"
            @change="applyFilters"
          >
            <SelectOption v-for="t in EVENT_OPTIONS" :key="t" :value="t">
              {{ t }}
            </SelectOption>
          </Select>
        </FormItem>

        <FormItem label="Sender">
          <Input
            v-model:value="filters.sender"
            allow-clear
            placeholder="contains…"
            style="width: 200px"
            @press-enter="applyFilters"
          />
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
            style="width: 280px"
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
          <template v-if="column.key === 'event_type'">
            <Tag :color="eventColor(record.event_type)">
              {{ record.event_type }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'mail_class'">
            <Tag v-if="record.mail_class" color="purple">
              {{ record.mail_class }}
            </Tag>
            <span v-else style="color: var(--ant-color-text-quaternary)">—</span>
          </template>
          <template v-else-if="column.key === 'vmta'">
            <Tag v-if="record.vmta" color="geekblue">
              {{ record.vmta }}
            </Tag>
            <span v-else style="color: var(--ant-color-text-quaternary)">—</span>
          </template>
          <template v-else-if="column.key === 'message_id'">
            <a
              v-if="record.message_id"
              class="msgid-link"
              :title="filters.message_id === record.message_id
                ? 'Click to clear filter'
                : 'Filter to all events for this message'"
              @click="filterToMessage(record.message_id)"
            >
              {{ record.message_id }}
            </a>
            <span v-else style="color: var(--ant-color-text-quaternary)">—</span>
          </template>
        </template>
      </Table>
    </Card>
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
</style>
