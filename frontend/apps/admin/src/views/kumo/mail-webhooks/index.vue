<script lang="ts" setup>
import type { MailWebhook } from '#/api/kumo';

import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Alert,
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
  Typography,
} from 'ant-design-vue';

import { mailWebhooksApi } from '#/api/kumo';

defineOptions({ name: 'MailWebhooks' });

const items = ref<MailWebhook[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);
const editingId = ref<null | number>(null);

const form = reactive<{
  address: string;
  enabled: boolean;
  name: string;
  secret: string;
  url: string;
}>({ address: '', enabled: true, name: '', secret: '', url: '' });

const secretAlreadySet = ref(false);

const columns = [
  { title: 'Name', dataIndex: 'name', key: 'name', width: 180 },
  { title: 'Recipient', dataIndex: 'address', key: 'address', width: 240 },
  { title: 'Endpoint', dataIndex: 'url', key: 'url' },
  { title: 'Signed', key: 'signed', width: 90 },
  { title: 'Enabled', dataIndex: 'enabled', key: 'enabled', width: 90 },
  { title: 'Actions', key: 'actions', width: 160 },
];

async function load() {
  loading.value = true;
  try {
    const resp = await mailWebhooksApi.list();
    items.value = resp.items ?? [];
  } catch {
    items.value = [];
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editingId.value = null;
  secretAlreadySet.value = false;
  form.name = '';
  form.address = '';
  form.url = '';
  form.secret = '';
  form.enabled = true;
  drawerOpen.value = true;
}

function openEdit(record: Record<string, any>) {
  const w = record as MailWebhook;
  editingId.value = w.id ?? null;
  secretAlreadySet.value = Boolean(w.secret_set);
  form.name = w.name;
  form.address = w.address;
  form.url = w.url;
  form.secret = ''; // write-only; blank keeps the stored one
  form.enabled = w.enabled ?? true;
  drawerOpen.value = true;
}

async function submit() {
  if (!form.name.trim() || !form.address.trim() || !form.url.trim()) {
    message.warning('Name, recipient and endpoint are required');
    return;
  }
  submitting.value = true;
  try {
    const payload: MailWebhook = {
      name: form.name.trim(),
      address: form.address.trim(),
      url: form.url.trim(),
      enabled: form.enabled,
    };
    if (form.secret.trim()) {
      payload.secret = form.secret.trim();
    }
    if (editingId.value === null) {
      await mailWebhooksApi.create(payload);
      message.success('Webhook created');
    } else {
      await mailWebhooksApi.update(editingId.value, payload);
      message.success('Webhook updated');
    }
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function removeRow(id: number) {
  await mailWebhooksApi.remove(id);
  await load();
}

onMounted(load);
</script>

<template>
  <Page
    title="Mail Webhooks"
    description="Forward inbound mail to an HTTP endpoint. When KumoMTA receives a message for the recipient, the raw message is POSTed to the endpoint."
  >
    <Alert
      type="info"
      show-icon
      class="mb-3"
      message="Recipient is an exact address (support@kmx.jobs.bg) or a bare domain (support.kmx.jobs.bg) for a catch-all. The recipient's domain MX must point at this KumoMTA. Changes take effect after Policy → Apply. The POST carries the raw message (Content-Type: message/rfc822) with X-Iris-Recipient / X-Iris-Message-Id headers; set a secret to add an X-Iris-Signature (HMAC-SHA256)."
    />

    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">New webhook</Button>
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
          <template v-if="column.key === 'address'">
            <Typography.Text code>{{ record.address }}</Typography.Text>
          </template>
          <template v-else-if="column.key === 'signed'">
            <Tag :color="record.secret_set ? 'green' : 'default'">
              {{ record.secret_set ? 'HMAC' : 'No' }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'enabled'">
            <Tag :color="record.enabled ? 'green' : 'default'">
              {{ record.enabled ? 'Yes' : 'No' }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Space>
              <Button size="small" @click="openEdit(record)">Edit</Button>
              <Popconfirm
                title="Delete this webhook?"
                ok-text="Delete"
                ok-type="danger"
                @confirm="removeRow(record.id)"
              >
                <Button danger size="small">Delete</Button>
              </Popconfirm>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="drawerOpen"
      :title="editingId === null ? 'New mail webhook' : 'Edit mail webhook'"
      width="540"
      :destroy-on-close="true"
    >
      <Form :model="form" layout="vertical">
        <FormItem label="Name" name="name" :rules="[{ required: true, message: 'Required' }]">
          <Input
            v-model:value="form.name"
            :disabled="editingId !== null"
            placeholder="e.g. support-inbox"
          />
        </FormItem>
        <FormItem label="Recipient (email or domain)" name="address">
          <Input v-model:value="form.address" placeholder="support@kmx.jobs.bg" />
        </FormItem>
        <FormItem label="HTTP endpoint" name="url">
          <Input v-model:value="form.url" placeholder="https://example.com/inbound-mail" />
        </FormItem>
        <FormItem label="Signing secret (optional)" name="secret">
          <Input.Password
            v-model:value="form.secret"
            :placeholder="secretAlreadySet ? '•••••• set — leave blank to keep' : 'HMAC-SHA256 key'"
          />
          <span style="color: var(--ant-color-text-tertiary)">
            When set, the POST includes an X-Iris-Signature header so your
            endpoint can verify the request came from iris.
          </span>
        </FormItem>
        <FormItem label="Enabled" name="enabled">
          <Switch v-model:checked="form.enabled" />
        </FormItem>
      </Form>
      <template #extra>
        <Space>
          <Button @click="drawerOpen = false">Cancel</Button>
          <Button type="primary" :loading="submitting" @click="submit">Save</Button>
        </Space>
      </template>
    </Drawer>
  </Page>
</template>
