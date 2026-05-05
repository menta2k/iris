<script lang="ts" setup>
import type { Vmta } from '#/api/kumo';

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

import { vmtasApi } from '#/api/kumo';

defineOptions({ name: 'Vmtas' });

const items = ref<Vmta[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);

const form = reactive({
  name: '',
  source_ips_text: '',
  helo_name: '',
  max_connections: 50,
  provider_profile: '',
});

const columns = [
  { title: 'Name', dataIndex: 'name', key: 'name', width: 200 },
  { title: 'HELO', dataIndex: 'helo_name', key: 'helo_name', width: 220 },
  { title: 'Source IPs', dataIndex: 'source_ips', key: 'source_ips' },
  {
    title: 'Max conns',
    dataIndex: 'max_connections',
    key: 'max_connections',
    width: 110,
    align: 'right' as const,
  },
  {
    title: 'Profile',
    dataIndex: 'provider_profile',
    key: 'provider_profile',
    width: 150,
  },
  { title: 'Actions', key: 'actions', width: 120 },
];

async function load() {
  loading.value = true;
  try {
    const r = await vmtasApi.list();
    items.value = r.items ?? [];
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  form.name = '';
  form.source_ips_text = '';
  form.helo_name = '';
  form.max_connections = 50;
  form.provider_profile = '';
  drawerOpen.value = true;
}

async function submit() {
  if (!form.name || !form.helo_name) {
    message.warning('Name and HELO are required');
    return;
  }
  submitting.value = true;
  try {
    const ips = form.source_ips_text
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    await vmtasApi.create({
      name: form.name.trim(),
      helo_name: form.helo_name.trim(),
      source_ips: ips,
      max_connections: form.max_connections,
      provider_profile: form.provider_profile || undefined,
    });
    message.success('VMTA created');
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function removeRow(id: string) {
  await vmtasApi.remove(id);
  message.success('VMTA removed');
  await load();
}

onMounted(load);
</script>

<template>
  <Page title="Virtual MTAs" description="Outbound source IP pools and HELO identity">
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">New VMTA</Button>
        <Button :loading="loading" @click="load">Refresh</Button>
      </Space>

      <Table
        :columns="columns"
        :data-source="items"
        :loading="loading"
        :pagination="{ pageSize: 20 }"
        row-key="id"
        size="middle"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'source_ips'">
            <Space wrap>
              <Tag v-for="ip in record.source_ips" :key="ip">{{ ip }}</Tag>
            </Space>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Popconfirm
              title="Delete this VMTA?"
              ok-text="Delete"
              ok-type="danger"
              @confirm="removeRow(record.id)"
            >
              <Button danger size="small">Delete</Button>
            </Popconfirm>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="drawerOpen"
      title="New Virtual MTA"
      width="500"
      :destroy-on-close="true"
    >
      <Form :model="form" layout="vertical">
        <FormItem
          label="Name"
          name="name"
          :rules="[{ required: true, message: 'Name is required' }]"
        >
          <Input v-model:value="form.name" placeholder="vmta-bulk-1" />
        </FormItem>
        <FormItem
          label="HELO name"
          name="helo_name"
          :rules="[{ required: true, message: 'HELO name is required' }]"
        >
          <Input v-model:value="form.helo_name" placeholder="mta1.example.com" />
        </FormItem>
        <FormItem label="Source IPs" name="source_ips_text">
          <Input.TextArea
            v-model:value="form.source_ips_text"
            :rows="3"
            placeholder="One IP per line or comma-separated"
          />
        </FormItem>
        <FormItem label="Max connections" name="max_connections">
          <InputNumber v-model:value="form.max_connections" :min="1" :max="10_000" />
        </FormItem>
        <FormItem label="Provider profile" name="provider_profile">
          <Input v-model:value="form.provider_profile" placeholder="(optional)" />
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
