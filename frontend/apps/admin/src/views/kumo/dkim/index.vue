<script lang="ts" setup>
import type { DkimIdentity } from '#/api/kumo';

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
  Modal,
  Popconfirm,
  Radio,
  RadioGroup,
  Select,
  SelectOption,
  Space,
  Table,
  Tag,
  Typography,
} from 'ant-design-vue';

import { dkimApi } from '#/api/kumo';

defineOptions({ name: 'Dkim' });

const items = ref<DkimIdentity[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);
const publicKeyOpen = ref(false);
const selectedKey = ref<DkimIdentity | null>(null);

type DkimAlgorithm = 'ed25519' | 'rsa-2048' | 'rsa-4096';
type CreateMode = 'generate' | 'import';

const form = reactive({
  mode: 'generate' as CreateMode,
  domain: '',
  selector: 'kumo',
  algorithm: 'ed25519' as DkimAlgorithm,
  private_key_pem: '',
});

const columns = [
  { title: 'Domain', dataIndex: 'domain', key: 'domain' },
  { title: 'Selector', dataIndex: 'selector', key: 'selector', width: 140 },
  { title: 'Algorithm', dataIndex: 'algorithm', key: 'algorithm', width: 130 },
  { title: 'Status', dataIndex: 'active', key: 'active', width: 110 },
  { title: 'Actions', key: 'actions', width: 280 },
];

async function load() {
  loading.value = true;
  try {
    const r = await dkimApi.list();
    items.value = r.items ?? [];
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  form.mode = 'generate';
  form.domain = '';
  form.selector = 'kumo';
  form.algorithm = 'ed25519';
  form.private_key_pem = '';
  drawerOpen.value = true;
}

async function submit() {
  if (!form.domain) {
    message.warning('Domain is required');
    return;
  }
  if (form.mode === 'import' && !form.private_key_pem.trim()) {
    message.warning('Paste a PEM-encoded private key to import');
    return;
  }
  submitting.value = true;
  try {
    await dkimApi.create({
      domain: form.domain.trim(),
      selector: form.selector.trim(),
      algorithm: form.algorithm,
      private_key_pem:
        form.mode === 'import' ? form.private_key_pem.trim() : undefined,
    });
    message.success(
      form.mode === 'import' ? 'DKIM key imported' : 'DKIM identity created',
    );
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function rotateKey(id: string) {
  await dkimApi.rotate(id);
  message.success('Key rotated');
  await load();
}

async function removeRow(id: string) {
  await dkimApi.remove(id);
  message.success('DKIM identity removed');
  await load();
}

function showPublicKey(item: Record<string, any>) {
  selectedKey.value = item as DkimIdentity;
  publicKeyOpen.value = true;
}

onMounted(load);
</script>

<template>
  <Page title="DKIM Identities" description="Domain Keys Identified Mail signing keys">
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">New identity</Button>
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
          <template v-if="column.key === 'algorithm'">
            <Tag>{{ record.algorithm }}</Tag>
          </template>
          <template v-else-if="column.key === 'active'">
            <Tag :color="record.active ? 'green' : 'default'">
              {{ record.active ? 'Active' : 'Inactive' }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Space>
              <Button size="small" @click="showPublicKey(record)">
                Public key
              </Button>
              <Popconfirm
                title="Rotate the signing key for this identity?"
                ok-text="Rotate"
                @confirm="rotateKey(record.id)"
              >
                <Button size="small">Rotate</Button>
              </Popconfirm>
              <Popconfirm
                title="Delete this DKIM identity?"
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
      title="New DKIM identity"
      width="560"
      :destroy-on-close="true"
    >
      <Form :model="form" layout="vertical">
        <FormItem label="Mode" name="mode">
          <RadioGroup v-model:value="form.mode" button-style="solid">
            <Radio.Button value="generate">Generate new keypair</Radio.Button>
            <Radio.Button value="import">Import existing key</Radio.Button>
          </RadioGroup>
        </FormItem>

        <FormItem
          label="Domain"
          name="domain"
          :rules="[{ required: true, message: 'Domain is required' }]"
        >
          <Input v-model:value="form.domain" placeholder="example.com" />
        </FormItem>

        <FormItem label="Selector" name="selector">
          <Input v-model:value="form.selector" />
        </FormItem>

        <FormItem label="Algorithm" name="algorithm">
          <Select v-model:value="form.algorithm">
            <SelectOption value="ed25519">ed25519</SelectOption>
            <SelectOption value="rsa-2048">rsa-2048</SelectOption>
            <SelectOption value="rsa-4096">rsa-4096</SelectOption>
          </Select>
          <span style="color: var(--ant-color-text-tertiary)">
            <template v-if="form.mode === 'import'">
              Must match the imported key. Mismatch is rejected server-side.
            </template>
            <template v-else>
              ed25519 produces shorter DNS records; rsa-2048 has the widest verifier support.
            </template>
          </span>
        </FormItem>

        <FormItem
          v-if="form.mode === 'import'"
          label="Private key (PEM)"
          name="private_key_pem"
          :rules="[{ required: true, message: 'Paste the private key PEM' }]"
        >
          <Input.TextArea
            v-model:value="form.private_key_pem"
            :rows="10"
            placeholder="-----BEGIN PRIVATE KEY-----
...
-----END PRIVATE KEY-----"
            style="font-family: ui-monospace, 'Menlo', 'Consolas', monospace"
          />
          <span style="color: var(--ant-color-text-tertiary)">
            PKCS#8 ("PRIVATE KEY") or legacy PKCS#1 ("RSA PRIVATE KEY") accepted.
            The matching public key is derived server-side.
          </span>
        </FormItem>
      </Form>
      <template #extra>
        <Space>
          <Button @click="drawerOpen = false">Cancel</Button>
          <Button type="primary" :loading="submitting" @click="submit">
            {{ form.mode === 'import' ? 'Import' : 'Generate' }}
          </Button>
        </Space>
      </template>
    </Drawer>

    <Modal
      v-model:open="publicKeyOpen"
      :title="`${selectedKey?.domain ?? ''} (${selectedKey?.selector ?? ''})`"
      width="640"
      :footer="null"
    >
      <Typography.Paragraph copyable code style="white-space: pre-wrap; word-break: break-all">
        {{ selectedKey?.public_key_pem || '— no public key on record —' }}
      </Typography.Paragraph>
    </Modal>
  </Page>
</template>
