<script lang="ts" setup>
import type { Bounce } from '#/api/kumo';

import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Button,
  Card,
  Drawer,
  Form,
  FormItem,
  Input,
  InputNumber,
  message,
  Popconfirm,
  Space,
  Table,
  Tag,
} from 'ant-design-vue';

import { bouncesApi } from '#/api/kumo';

defineOptions({ name: 'Bounces' });

const items = ref<Bounce[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);

const form = reactive({
  domain: '',
  tenant: '',
  campaign: '',
  duration_seconds: 3600,
});

const columns = [
  { title: 'Domain', dataIndex: 'domain', key: 'domain' },
  { title: 'Tenant', dataIndex: 'tenant', key: 'tenant' },
  { title: 'Campaign', dataIndex: 'campaign', key: 'campaign' },
  {
    title: 'Duration (s)',
    dataIndex: 'duration_seconds',
    key: 'duration_seconds',
    width: 130,
    align: 'right' as const,
  },
  { title: 'Expires', dataIndex: 'expires_at', key: 'expires_at', width: 200 },
  { title: 'Actions', key: 'actions', width: 110 },
];

async function load() {
  loading.value = true;
  try {
    const r = await bouncesApi.list();
    items.value = r.items ?? [];
  } catch {
    items.value = [];
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  form.domain = '';
  form.tenant = '';
  form.campaign = '';
  form.duration_seconds = 3600;
  drawerOpen.value = true;
}

async function submit() {
  if (!form.domain && !form.tenant && !form.campaign) {
    message.warning('Specify at least one of domain, tenant, or campaign');
    return;
  }
  submitting.value = true;
  try {
    await bouncesApi.create({
      domain: form.domain.trim() || undefined,
      tenant: form.tenant.trim() || undefined,
      campaign: form.campaign.trim() || undefined,
      duration_seconds: form.duration_seconds,
    });
    message.success('Bounce rule created');
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function removeRow(id: string) {
  await bouncesApi.remove(id);
  await load();
}

onMounted(load);
</script>

<template>
  <Page title="Bounces" description="Auto-bounce rules with expiry">
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">New bounce rule</Button>
        <Button :loading="loading" @click="load">Refresh</Button>
      </Space>

      <Table
        :columns="columns"
        :data-source="items"
        :loading="loading"
        :pagination="{ pageSize: 25 }"
        row-key="id"
        size="middle"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'domain' && !record.domain">
            <Tag>any</Tag>
          </template>
          <template v-else-if="column.key === 'tenant' && !record.tenant">
            <Tag>any</Tag>
          </template>
          <template v-else-if="column.key === 'campaign' && !record.campaign">
            <Tag>any</Tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Popconfirm
              title="Cancel this bounce rule?"
              ok-text="Cancel"
              ok-type="danger"
              @confirm="removeRow(record.id)"
            >
              <Button danger size="small">Cancel</Button>
            </Popconfirm>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="drawerOpen"
      title="New bounce rule"
      width="420"
      :destroy-on-close="true"
    >
      <Form :model="form" layout="vertical">
        <FormItem label="Domain" name="domain">
          <Input v-model:value="form.domain" placeholder="example.com (optional)" />
        </FormItem>
        <FormItem label="Tenant" name="tenant">
          <Input v-model:value="form.tenant" placeholder="(optional)" />
        </FormItem>
        <FormItem label="Campaign" name="campaign">
          <Input v-model:value="form.campaign" placeholder="(optional)" />
        </FormItem>
        <FormItem
          label="Duration (seconds)"
          name="duration_seconds"
          :rules="[{ required: true, message: 'Duration is required' }]"
        >
          <InputNumber v-model:value="form.duration_seconds" :min="60" :step="60" />
        </FormItem>
      </Form>
      <template #extra>
        <Space>
          <Button @click="drawerOpen = false">Cancel</Button>
          <Button type="primary" :loading="submitting" @click="submit">
            Save
          </Button>
        </Space>
      </template>
    </Drawer>
  </Page>
</template>
