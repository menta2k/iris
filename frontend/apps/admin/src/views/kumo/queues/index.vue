<script lang="ts" setup>
import type { QueueItem } from '#/api/kumo';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Button,
  Card,
  Input,
  message,
  Popconfirm,
  Space,
  Table,
  Tag,
} from 'ant-design-vue';

import { queuesApi } from '#/api/kumo';

defineOptions({ name: 'Queues' });

const items = ref<QueueItem[]>([]);
const loading = ref(false);
const filter = ref('');

const columns = [
  { title: 'Queue', dataIndex: 'name', key: 'name' },
  {
    title: 'Size',
    dataIndex: 'queue_size',
    key: 'queue_size',
    width: 110,
    align: 'right' as const,
    sorter: (a: QueueItem, b: QueueItem) => a.queue_size - b.queue_size,
  },
  {
    title: 'Delivered',
    dataIndex: 'delivered',
    key: 'delivered',
    width: 120,
    align: 'right' as const,
    sorter: (a: QueueItem, b: QueueItem) => a.delivered - b.delivered,
  },
  {
    title: 'Failed',
    dataIndex: 'failed',
    key: 'failed',
    width: 110,
    align: 'right' as const,
    sorter: (a: QueueItem, b: QueueItem) => a.failed - b.failed,
  },
  { title: 'Status', dataIndex: 'suspended', key: 'suspended', width: 130 },
  { title: 'Actions', key: 'actions', width: 260 },
];

async function load() {
  loading.value = true;
  try {
    const r = await queuesApi.list({ filter: filter.value, limit: 200 });
    items.value = r.items ?? [];
  } finally {
    loading.value = false;
  }
}

async function suspendQueue(name: string) {
  await queuesApi.suspend(name);
  message.success(`Suspended ${name}`);
  await load();
}

async function resumeQueue(name: string) {
  await queuesApi.resume(name);
  message.success(`Resumed ${name}`);
  await load();
}

async function bounceQueue(name: string) {
  await queuesApi.bounce(name);
  message.success(`Bounced ${name}`);
  await load();
}

onMounted(load);
</script>

<template>
  <Page title="Queues" description="Operational view of Kumo MTA queues">
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Input
          v-model:value="filter"
          name="queue-filter"
          allow-clear
          placeholder="Filter queues..."
          style="width: 260px"
          @press-enter="load"
        />
        <Button type="primary" :loading="loading" @click="load">Refresh</Button>
      </Space>

      <Table
        :columns="columns"
        :data-source="items"
        :loading="loading"
        :pagination="{ pageSize: 25, showSizeChanger: true }"
        row-key="name"
        size="middle"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'suspended'">
            <Tag :color="record.suspended ? 'orange' : 'green'">
              {{ record.suspended ? 'Suspended' : 'Active' }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Space>
              <Button
                v-if="!record.suspended"
                size="small"
                @click="suspendQueue(record.name)"
              >
                Suspend
              </Button>
              <Button
                v-else
                size="small"
                type="primary"
                @click="resumeQueue(record.name)"
              >
                Resume
              </Button>
              <Popconfirm
                :title="`Bounce all messages in ${record.name}?`"
                ok-text="Bounce"
                ok-type="danger"
                cancel-text="Cancel"
                @confirm="bounceQueue(record.name)"
              >
                <Button size="small" danger>Bounce</Button>
              </Popconfirm>
            </Space>
          </template>
        </template>
      </Table>
    </Card>
  </Page>
</template>
