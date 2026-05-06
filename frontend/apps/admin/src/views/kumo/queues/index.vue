<script lang="ts" setup>
import type { QueueItem, ScheduledMessage } from '#/api/kumo';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Button,
  Card,
  Input,
  message,
  Modal,
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

const inspect = ref({
  open: false,
  loading: false,
  queue: '',
  rows: [] as ScheduledMessage[],
});

const messageColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 240, ellipsis: true },
  { title: 'Recipient', dataIndex: 'recipient', key: 'recipient' },
  { title: 'Sender', dataIndex: 'sender', key: 'sender' },
  {
    title: 'Attempts',
    dataIndex: 'num_attempts',
    key: 'num_attempts',
    width: 100,
    align: 'right' as const,
  },
  { title: 'Due', dataIndex: 'due_at', key: 'due_at', width: 200 },
];

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
  {
    title: 'Deferred',
    dataIndex: 'deferred',
    key: 'deferred',
    width: 120,
    align: 'right' as const,
    sorter: (a: QueueItem, b: QueueItem) =>
      (a.deferred ?? 0) - (b.deferred ?? 0),
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

async function openMessages(name: string) {
  inspect.value = { open: true, loading: true, queue: name, rows: [] };
  try {
    const r = await queuesApi.inspect(name, 100);
    inspect.value.rows = r.items ?? [];
  } catch {
    // Service-level errors already surface via the global request
    // interceptor; just clear the loading state so the modal isn't stuck.
  } finally {
    inspect.value.loading = false;
  }
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
                v-if="(record.deferred ?? 0) > 0"
                size="small"
                @click="openMessages(record.name)"
              >
                Messages
              </Button>
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

    <Modal
      v-model:open="inspect.open"
      :title="`Deferred messages — ${inspect.queue}`"
      :footer="null"
      width="960px"
      destroy-on-close
    >
      <Table
        :columns="messageColumns"
        :data-source="inspect.rows"
        :loading="inspect.loading"
        :pagination="{ pageSize: 20 }"
        row-key="id"
        size="small"
      />
    </Modal>
  </Page>
</template>
