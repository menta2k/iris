<script lang="ts" setup>
import type { ListenerDomain } from '#/api/kumo';

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
  Space,
  Switch,
  Table,
  Tag,
} from 'ant-design-vue';

import { listenerDomainsApi } from '#/api/kumo';

defineOptions({ name: 'ListenerDomains' });

const items = ref<ListenerDomain[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);

const form = reactive({
  domain: '',
  relay_to: '',
  enabled: true,
});

const columns = [
  { title: 'Domain', dataIndex: 'domain', key: 'domain' },
  { title: 'Relay to', dataIndex: 'relay_to', key: 'relay_to' },
  { title: 'Enabled', dataIndex: 'enabled', key: 'enabled', width: 110 },
  { title: 'Actions', key: 'actions', width: 110 },
];

async function load() {
  loading.value = true;
  try {
    const r = await listenerDomainsApi.list();
    items.value = r.items ?? [];
  } catch {
    items.value = [];
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  form.domain = '';
  form.relay_to = '';
  form.enabled = true;
  drawerOpen.value = true;
}

async function submit() {
  if (!form.domain) {
    message.warning('Domain is required');
    return;
  }
  submitting.value = true;
  try {
    await listenerDomainsApi.create({
      domain: form.domain.trim(),
      relay_to: form.relay_to.trim() || undefined,
      enabled: form.enabled,
    });
    message.success('Domain added');
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function removeRow(id: string) {
  await listenerDomainsApi.remove(id);
  await load();
}

onMounted(load);
</script>

<template>
  <Page title="Listener Domains" description="Per-domain inbound listener configuration">
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">Add domain</Button>
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
          <template v-if="column.key === 'enabled'">
            <Tag :color="record.enabled ? 'green' : 'default'">
              {{ record.enabled ? 'Yes' : 'No' }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Popconfirm
              title="Remove this domain?"
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
      title="Add listener domain"
      width="420"
      :destroy-on-close="true"
    >
      <Form :model="form" layout="vertical">
        <FormItem
          label="Domain"
          name="domain"
          :rules="[{ required: true, message: 'Domain is required' }]"
        >
          <Input v-model:value="form.domain" placeholder="example.com" />
        </FormItem>
        <FormItem label="Relay to" name="relay_to">
          <Input v-model:value="form.relay_to" placeholder="(optional)" />
        </FormItem>
        <FormItem label="Enabled" name="enabled">
          <Switch v-model:checked="form.enabled" />
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
