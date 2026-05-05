<script lang="ts" setup>
import type { FeedbackReport } from '#/api/kumo';

import { onMounted, onUnmounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useIntervalFn } from '@vueuse/core';

import {
  Button,
  Card,
  Descriptions,
  DescriptionsItem,
  Drawer,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'ant-design-vue';

import { feedbackApi } from '#/api/kumo';

defineOptions({ name: 'FeedbackReports' });

const items = ref<FeedbackReport[]>([]);
const loading = ref(false);
const autoRefresh = ref(false);
const detailOpen = ref(false);
const selected = ref<FeedbackReport | null>(null);

const columns = [
  { title: 'Received', dataIndex: 'received_at', key: 'received_at', width: 200 },
  {
    title: 'Type',
    dataIndex: 'feedback_type',
    key: 'feedback_type',
    width: 130,
  },
  {
    title: 'Original recipient',
    dataIndex: 'original_recipient',
    key: 'original_recipient',
  },
  {
    title: 'Reporting MTA',
    dataIndex: 'reporting_mta',
    key: 'reporting_mta',
  },
  { title: '', key: 'actions', width: 100 },
];

async function load() {
  loading.value = true;
  try {
    const r = await feedbackApi.list({ limit: 200 });
    items.value = r.items ?? [];
  } finally {
    loading.value = false;
  }
}

function typeColor(t: string): string {
  switch (t) {
    case 'abuse':
    case 'fraud':
    case 'virus': {
      return 'red';
    }
    case 'opt-out': {
      return 'orange';
    }
    case 'auth-failure': {
      return 'volcano';
    }
    default: {
      return 'default';
    }
  }
}

function showDetail(item: Record<string, any>) {
  selected.value = item as FeedbackReport;
  detailOpen.value = true;
}

const { pause, resume } = useIntervalFn(load, 15_000, { immediate: false });

function toggleAuto(on: boolean | string | number) {
  const enabled = Boolean(on);
  autoRefresh.value = enabled;
  if (enabled) resume();
  else pause();
}

onMounted(load);
onUnmounted(pause);
</script>

<template>
  <Page title="Feedback Reports" description="ARF-format feedback loop reports from receiving MTAs">
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button :loading="loading" @click="load">Refresh</Button>
        <span>Auto-refresh (15s)</span>
        <Switch :checked="autoRefresh" @change="toggleAuto" />
      </Space>

      <Table
        :columns="columns"
        :data-source="items"
        :loading="loading"
        :pagination="{ pageSize: 50 }"
        row-key="id"
        size="middle"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'feedback_type'">
            <Tag :color="typeColor(record.feedback_type)">
              {{ record.feedback_type }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Button size="small" @click="showDetail(record)">View</Button>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="detailOpen"
      title="Report detail"
      width="640"
      :destroy-on-close="true"
    >
      <Descriptions v-if="selected" :column="1" bordered size="small">
        <DescriptionsItem label="Received">
          {{ selected.received_at }}
        </DescriptionsItem>
        <DescriptionsItem label="Type">
          <Tag :color="typeColor(selected.feedback_type)">
            {{ selected.feedback_type }}
          </Tag>
        </DescriptionsItem>
        <DescriptionsItem label="Recipient">
          {{ selected.original_recipient }}
        </DescriptionsItem>
        <DescriptionsItem label="Reporting MTA">
          {{ selected.reporting_mta }}
        </DescriptionsItem>
      </Descriptions>
      <Typography.Title v-if="selected?.raw" :level="5" style="margin-top: 24px">
        Raw report
      </Typography.Title>
      <Typography.Paragraph
        v-if="selected?.raw"
        copyable
        code
        style="white-space: pre-wrap; word-break: break-all"
      >
        {{ selected.raw }}
      </Typography.Paragraph>
    </Drawer>
  </Page>
</template>
