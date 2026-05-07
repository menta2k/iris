<script lang="ts" setup>
import type { AcmeCertificate, Listener } from '#/api/kumo';

import { computed, onMounted, reactive, ref, watch } from 'vue';

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
  Radio,
  RadioGroup,
  Select,
  Space,
  Switch,
  Table,
  Tag,
} from 'ant-design-vue';

import { acmeApi, listenersApi } from '#/api/kumo';

defineOptions({ name: 'Listener' });

const items = ref<Listener[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);
const editingId = ref<null | number>(null);

// ACME-issued certs available as TLS sources for this listener. Loaded
// once per page mount; refreshed when the drawer opens so newly-issued
// certs show up without a full page reload.
const acmeCerts = ref<AcmeCertificate[]>([]);
// Per-drawer-open: which TLS source the operator picked. Not persisted —
// the backend only stores tls_cert_pem_path / tls_key_pem_path.
const tlsSource = ref<'acme' | 'manual'>('manual');
// id of the selected ACME cert in `acme` mode. Watched below to
// auto-fill the two path fields whenever the operator picks one.
const selectedAcmeId = ref<null | number>(null);

const blank: Listener = {
  name: '',
  listen_addr: '0.0.0.0:25',
  hostname: '',
  tls_enabled: false,
  tls_cert_pem_path: '',
  tls_key_pem_path: '',
  require_auth: false,
  max_message_size: 0,
};

const form = reactive<Listener>({ ...blank });

const columns = [
  { title: 'Name', dataIndex: 'name', key: 'name', width: 160 },
  { title: 'Listen', dataIndex: 'listen_addr', key: 'listen_addr', width: 180 },
  { title: 'Hostname', dataIndex: 'hostname', key: 'hostname' },
  { title: 'TLS', dataIndex: 'tls_enabled', key: 'tls_enabled', width: 90 },
  {
    title: 'AUTH',
    dataIndex: 'require_auth',
    key: 'require_auth',
    width: 90,
  },
  { title: 'Actions', key: 'actions', width: 180 },
];

async function load() {
  loading.value = true;
  try {
    const r = await listenersApi.list();
    items.value = r.items ?? [];
  } finally {
    loading.value = false;
  }
}

// Refresh the cert dropdown lazily so the drawer always sees the
// latest issuance without forcing a full page reload after the operator
// hops over to /security/certificates and back.
async function loadAcmeCerts() {
  try {
    const r = await acmeApi.listCertificates();
    acmeCerts.value = (r.items ?? []).filter(
      (c) => c.status === 'issued' && c.cert_pem_path && c.key_pem_path,
    );
  } catch {
    acmeCerts.value = [];
  }
}

// Detect whether an editing row's existing paths match an ACME-mirrored
// cert (under /opt/kumomta/etc/tls/<domain>/...). When they do we open
// the drawer in `acme` mode and pre-select that cert; otherwise we
// fall back to `manual` so the operator's hand-curated paths aren't
// silently rewritten.
function detectTlsSource(row: Listener) {
  if (!row.tls_enabled || !row.tls_cert_pem_path) {
    tlsSource.value = 'manual';
    selectedAcmeId.value = null;
    return;
  }
  const match = acmeCerts.value.find(
    (c) =>
      c.cert_pem_path === row.tls_cert_pem_path &&
      c.key_pem_path === row.tls_key_pem_path,
  );
  if (match?.id) {
    tlsSource.value = 'acme';
    selectedAcmeId.value = match.id;
  } else {
    tlsSource.value = 'manual';
    selectedAcmeId.value = null;
  }
}

async function openCreate() {
  Object.assign(form, blank);
  editingId.value = null;
  tlsSource.value = 'manual';
  selectedAcmeId.value = null;
  await loadAcmeCerts();
  drawerOpen.value = true;
}

async function openEdit(row: Listener) {
  Object.assign(form, blank, row);
  editingId.value = row.id ?? null;
  await loadAcmeCerts();
  detectTlsSource(form);
  drawerOpen.value = true;
}

// When the operator picks a cert in ACME mode, copy the cert's mirrored
// PEM paths onto the form fields. The backend's Listener model still
// stores plain paths — so renewals (which preserve the path, only
// rewrite contents) work transparently and the renderer doesn't need
// to know about ACME at all.
watch(selectedAcmeId, (id) => {
  if (tlsSource.value !== 'acme' || id == null) return;
  const cert = acmeCerts.value.find((c) => c.id === id);
  if (!cert) return;
  form.tls_cert_pem_path = cert.cert_pem_path ?? '';
  form.tls_key_pem_path = cert.key_pem_path ?? '';
});

// Switching back to manual clears the ACME selection but keeps the
// paths so the operator can edit them.
watch(tlsSource, (mode) => {
  if (mode === 'manual') selectedAcmeId.value = null;
});

// Helper for the dropdown label.
function labelForCert(c: AcmeCertificate): string {
  const sans = (c.alt_names ?? []).length > 0 ? ` (+${c.alt_names!.length} SAN)` : '';
  return `${c.domain}${sans}`;
}

const acmeCertsAvailable = computed(() => acmeCerts.value.length > 0);

async function submit() {
  if (!form.name.trim()) {
    message.warning('Name is required');
    return;
  }
  if (!form.listen_addr.includes(':')) {
    message.warning('Listen address must be host:port (e.g. 0.0.0.0:25)');
    return;
  }
  if (form.tls_enabled) {
    if (!form.tls_cert_pem_path?.trim() || !form.tls_key_pem_path?.trim()) {
      message.warning('TLS cert + key paths are required when TLS is enabled');
      return;
    }
  }
  submitting.value = true;
  try {
    if (editingId.value == null) {
      await listenersApi.create(form);
      message.success('Listener created');
    } else {
      await listenersApi.update(editingId.value, form);
      message.success('Listener updated');
    }
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function removeRow(id: number) {
  await listenersApi.remove(id);
  message.success('Deleted');
  await load();
}

onMounted(load);
</script>

<template>
  <Page
    title="Listeners"
    description="kumomta SMTP listeners. Each row renders into one kumo.start_esmtp_listener block — typically :25 for inbound and :587 for submission with auth."
  >
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">New listener</Button>
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
          <template v-if="column.key === 'tls_enabled'">
            <Tag v-if="record.tls_enabled" color="green">on</Tag>
            <Tag v-else color="default">off</Tag>
          </template>
          <template v-else-if="column.key === 'require_auth'">
            <Tag v-if="record.require_auth" color="blue">required</Tag>
            <Tag v-else color="default">none</Tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Space>
              <Button size="small" @click="openEdit(record as Listener)">Edit</Button>
              <Popconfirm
                :title="`Delete listener ${record.name}?`"
                ok-text="Delete"
                ok-type="danger"
                @confirm="removeRow((record as Listener).id!)"
              >
                <Button size="small" danger>Delete</Button>
              </Popconfirm>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="drawerOpen"
      :title="editingId == null ? 'New listener' : `Edit listener ${form.name}`"
      width="520"
      :destroy-on-close="true"
    >
      <Form :model="form" layout="vertical">
        <FormItem
          label="Name"
          name="name"
          :rules="[{ required: true, message: 'Name is required' }]"
        >
          <Input
            v-model:value="form.name"
            placeholder="smtp / submission / lmtp …"
            :disabled="editingId != null"
          />
          <div class="hint">
            Identifier in the rendered Lua. Must be 1–64 chars
            <code>[a-zA-Z0-9_.-]</code>. Immutable after create — to rename, delete and recreate.
          </div>
        </FormItem>

        <FormItem
          label="Listen address"
          name="listen_addr"
          :rules="[{ required: true, message: 'Listen address is required' }]"
        >
          <Input v-model:value="form.listen_addr" placeholder="0.0.0.0:25" />
          <div class="hint">
            <code>host:port</code>. Use <code>0.0.0.0:25</code> for inbound,
            <code>0.0.0.0:587</code> for submission. <code>:25</code> needs the kumomta
            container to have <code>cap_net_bind_service</code> (or run as root, the default).
          </div>
        </FormItem>

        <FormItem label="Hostname (HELO / banner)" name="hostname">
          <Input v-model:value="form.hostname" placeholder="mta.example.com" />
        </FormItem>

        <FormItem name="tls_enabled">
          <Space>
            <Switch v-model:checked="form.tls_enabled" />
            <span>TLS enabled (STARTTLS)</span>
          </Space>
        </FormItem>

        <template v-if="form.tls_enabled">
          <FormItem label="TLS source">
            <RadioGroup v-model:value="tlsSource">
              <Radio value="acme" :disabled="!acmeCertsAvailable">
                ACME-issued certificate
              </Radio>
              <Radio value="manual">Manual paths</Radio>
            </RadioGroup>
            <div v-if="!acmeCertsAvailable" class="hint">
              No issued ACME certificates found.
              <a href="/security/certificates">Issue one</a> on the
              Certificates page, then come back.
            </div>
          </FormItem>

          <template v-if="tlsSource === 'acme'">
            <FormItem
              label="Certificate"
              :rules="[{ required: true, message: 'Pick an issued certificate' }]"
            >
              <Select
                :value="selectedAcmeId ?? undefined"
                placeholder="Select an issued certificate…"
                style="width: 100%"
                @change="(v: any) => (selectedAcmeId = (v as null | number) ?? null)"
              >
                <Select.Option
                  v-for="c in acmeCerts"
                  :key="c.id"
                  :value="c.id"
                >
                  {{ labelForCert(c) }}
                </Select.Option>
              </Select>
              <div class="hint">
                Selecting a cert fills the two path fields below. The paths
                stay stable across renewals — kumomta re-reads the file
                contents on each epoch — so this picker only changes which
                cert this listener uses, not how renewal works.
              </div>
            </FormItem>
          </template>

          <FormItem label="TLS certificate path">
            <Input
              v-model:value="form.tls_cert_pem_path"
              :disabled="tlsSource === 'acme'"
              placeholder="/opt/kumomta/etc/tls/fullchain.pem"
            />
          </FormItem>
          <FormItem label="TLS private key path">
            <Input
              v-model:value="form.tls_key_pem_path"
              :disabled="tlsSource === 'acme'"
              placeholder="/opt/kumomta/etc/tls/privkey.pem"
            />
          </FormItem>
          <div class="hint">
            Paths inside the kumomta container. ACME certs land at
            <code>/opt/kumomta/etc/tls/&lt;domain&gt;/{fullchain,privkey}.pem</code>;
            for non-ACME deployments mount your own cert + key via volume.
          </div>
        </template>

        <FormItem name="require_auth">
          <Space>
            <Switch v-model:checked="form.require_auth" />
            <span>Require SMTP AUTH</span>
          </Space>
          <div class="hint">
            Recommended for the submission listener (port 587). Auth backend
            is configured in kumomta policy — iris just sets the flag.
          </div>
        </FormItem>

        <FormItem label="Max message size (bytes)">
          <InputNumber
            v-model:value="form.max_message_size"
            :min="0"
            :step="1024 * 1024"
            style="width: 200px"
          />
          <span class="hint">&nbsp;0 = use kumomta default</span>
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

<style scoped>
.hint {
  font-size: 12px;
  color: var(--ant-color-text-tertiary);
  margin-top: 4px;
}
.hint code {
  font-size: 12px;
}
</style>
