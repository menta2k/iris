<script lang="ts" setup>
import type { Suppression } from '#/api/kumo';

import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Button,
  Card,
  Drawer,
  Form,
  FormItem,
  Input,
  message,
  Popconfirm,
  Select,
  SelectOption,
  Space,
  Table,
} from 'ant-design-vue';

import { suppressionsApi } from '#/api/kumo';

defineOptions({ name: 'Suppressions' });

const items = ref<Suppression[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);

const form = reactive({
  address: '',
  scope: 'global',
  reason: '',
});

const columns = [
  { title: 'Address', dataIndex: 'address', key: 'address' },
  { title: 'Scope', dataIndex: 'scope', key: 'scope', width: 140 },
  { title: 'Reason', dataIndex: 'reason', key: 'reason' },
  { title: 'Created', dataIndex: 'created_at', key: 'created_at', width: 200 },
  { title: 'Actions', key: 'actions', width: 110 },
];

async function load() {
  loading.value = true;
  try {
    const r = await suppressionsApi.list({ limit: 500 });
    items.value = r.items ?? [];
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  form.address = '';
  form.scope = 'global';
  form.reason = '';
  drawerOpen.value = true;
}

async function submit() {
  if (!form.address) {
    message.warning('Address is required');
    return;
  }
  submitting.value = true;
  try {
    await suppressionsApi.create({
      address: form.address.trim(),
      scope: form.scope,
      reason: form.reason || undefined,
    });
    message.success('Suppression added');
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function removeRow(id: string) {
  await suppressionsApi.remove(id);
  message.success('Suppression removed');
  await load();
}

onMounted(load);
</script>

<template>
  <Page title="Suppressions" description="Suppress delivery to specific recipients or domains">
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">Add suppression</Button>
        <Button :loading="loading" @click="load">Refresh</Button>
      </Space>

      <Table
        :columns="columns"
        :data-source="items"
        :loading="loading"
        :pagination="{ pageSize: 25, showSizeChanger: true }"
        row-key="id"
        size="middle"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'actions'">
            <Popconfirm
              title="Remove this suppression?"
              ok-text="Remove"
              ok-type="danger"
              @confirm="removeRow(record.id)"
            >
              <Button danger size="small">Remove</Button>
            </Popconfirm>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="drawerOpen"
      title="Add suppression"
      width="420"
      :destroy-on-close="true"
    >
      <Form :model="form" layout="vertical">
        <FormItem
          label="Address"
          name="address"
          :rules="[{ required: true, message: 'Address is required' }]"
        >
          <Input
            v-model:value="form.address"
            placeholder="user@example.com or example.com"
          />
        </FormItem>
        <FormItem label="Scope" name="scope">
          <Select v-model:value="form.scope">
            <SelectOption value="global">global</SelectOption>
            <SelectOption value="domain">domain</SelectOption>
            <SelectOption value="recipient">recipient</SelectOption>
          </Select>
        </FormItem>
        <FormItem label="Reason" name="reason">
          <Input
            v-model:value="form.reason"
            placeholder="Why this entry was added"
          />
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
