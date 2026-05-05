<script lang="ts" setup>
import type { AuditEntry } from '#/api/kumo';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Card, Space, Table, Tag } from 'ant-design-vue';

import { auditApi } from '#/api/kumo';

defineOptions({ name: 'AuditLog' });

const items = ref<AuditEntry[]>([]);
const loading = ref(false);

const columns = [
  { title: 'When', dataIndex: 'at', key: 'at', width: 200 },
  { title: 'Operation', dataIndex: 'operation', key: 'operation', width: 200 },
  { title: 'Actor', dataIndex: 'actor_username', key: 'actor_username', width: 180 },
  { title: 'Resource', key: 'resource' },
  { title: 'Client IP', dataIndex: 'client_ip', key: 'client_ip', width: 160 },
  { title: 'Status', dataIndex: 'status_code', key: 'status_code', width: 100 },
  { title: 'Duration', dataIndex: 'duration_ms', key: 'duration_ms', width: 110 },
];

async function load() {
  loading.value = true;
  try {
    const r = await auditApi.list({ limit: 500 });
    items.value = r.items ?? [];
  } finally {
    loading.value = false;
  }
}

function statusColor(code: number): string {
  if (code >= 200 && code < 300) return 'green';
  if (code >= 400 && code < 500) return 'orange';
  if (code >= 500) return 'red';
  return 'default';
}

onMounted(load);
</script>

<template>
  <Page title="Audit Log" description="Every operation against the management API">
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button :loading="loading" @click="load">Refresh</Button>
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
          <template v-if="column.key === 'resource'">
            {{ record.resource_type }}
            <Tag v-if="record.resource_id">{{ record.resource_id }}</Tag>
          </template>
          <template v-else-if="column.key === 'status_code'">
            <Tag :color="statusColor(record.status_code)">{{ record.status_code }}</Tag>
          </template>
          <template v-else-if="column.key === 'duration_ms'">
            {{ record.duration_ms }} ms
          </template>
        </template>
      </Table>
    </Card>
  </Page>
</template>
