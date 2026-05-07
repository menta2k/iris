<script lang="ts" setup>
import type {
  AcmeCertificate,
  AcmeDnsProviderConfig,
} from '#/api/kumo';

import { computed, onMounted, reactive, ref } from 'vue';

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
  Radio,
  RadioGroup,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
} from 'ant-design-vue';

import { acmeApi } from '#/api/kumo';

defineOptions({ name: 'AcmeCertificates' });

const loading = ref(false);
const submitting = ref(false);
const items = ref<AcmeCertificate[]>([]);
const dnsProviders = ref<AcmeDnsProviderConfig[]>([]);

const drawerOpen = ref(false);
const renewingId = ref<null | number>(null);

const form = reactive({
  domain: '',
  alt_names: [] as string[],
  challenge_type: 'http-01' as 'dns-01' | 'http-01',
  dns_provider: '',
});

const columns = [
  { title: 'Domain', dataIndex: 'domain', key: 'domain', width: 240 },
  {
    title: 'SANs',
    dataIndex: 'alt_names',
    key: 'alt_names',
    width: 240,
  },
  {
    title: 'Challenge',
    dataIndex: 'challenge_type',
    key: 'challenge_type',
    width: 130,
  },
  { title: 'Status', dataIndex: 'status', key: 'status', width: 120 },
  { title: 'Expires', dataIndex: 'expires_at', key: 'expires_at', width: 180 },
  { title: 'Actions', key: 'actions', width: 220 },
];

const dnsProvidersConfigured = computed(() =>
  dnsProviders.value.map((c) => c.provider),
);

async function load() {
  loading.value = true;
  try {
    const [certs, providers] = await Promise.all([
      acmeApi.listCertificates(),
      acmeApi.listDnsProviderConfigs(),
    ]);
    items.value = certs.items ?? [];
    dnsProviders.value = providers.items ?? [];
  } finally {
    loading.value = false;
  }
}

function openIssue() {
  form.domain = '';
  form.alt_names = [];
  form.challenge_type = 'http-01';
  form.dns_provider = '';
  drawerOpen.value = true;
}

async function submit() {
  if (!form.domain.trim()) {
    message.warning('Domain is required');
    return;
  }
  if (form.challenge_type === 'dns-01' && !form.dns_provider) {
    message.warning(
      'DNS-01 needs a configured DNS provider. Set one up on the DNS Providers page first.',
    );
    return;
  }
  submitting.value = true;
  try {
    await acmeApi.issueCertificate({
      domain: form.domain.trim(),
      alt_names: form.alt_names,
      challenge_type: form.challenge_type,
      dns_provider:
        form.challenge_type === 'dns-01' ? form.dns_provider : undefined,
    });
    message.success(`Issued ${form.domain}`);
    drawerOpen.value = false;
    await load();
  } catch {
    // Backend already surfaces a typed error via the global request
    // interceptor; we just keep the drawer open so the operator can fix
    // the input.
  } finally {
    submitting.value = false;
  }
}

async function renew(id: number) {
  renewingId.value = id;
  try {
    await acmeApi.renewCertificate(id);
    message.success('Renewed');
    await load();
  } finally {
    renewingId.value = null;
  }
}

async function removeRow(id: number) {
  await acmeApi.removeCertificate(id);
  message.success('Removed');
  await load();
}

// --- column rendering helpers ----------------------------------------------

function statusColor(s: string): string {
  switch (s) {
    case 'failed':
      return 'red';
    case 'issued':
      return 'green';
    case 'pending':
    case 'renewing':
      return 'gold';
    default:
      return 'default';
  }
}

// "expires in N days" — short, glanceable, with a colour cue when close
// to expiry. Returns { text, color }.
function expiryHint(iso?: string): null | { color: string; text: string } {
  if (!iso) return null;
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return null;
  const days = Math.floor((t - Date.now()) / (24 * 3600 * 1000));
  if (days < 0) return { text: `expired ${-days}d ago`, color: 'red' };
  if (days < 7) return { text: `${days}d`, color: 'red' };
  if (days < 30) return { text: `${days}d`, color: 'orange' };
  return { text: `${days}d`, color: 'default' };
}

onMounted(load);
</script>

<template>
  <Page
    title="Certificates"
    description="ACME-issued TLS certificates. PEMs are mirrored to /opt/kumomta/etc/tls/<domain>/{fullchain.pem,privkey.pem}; reference those paths from the Listeners page to enable TLS on a kumomta listener."
  >
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openIssue">Issue certificate</Button>
        <Button :loading="loading" @click="load">Refresh</Button>
      </Space>

      <Alert
        v-if="!loading && items.length === 0"
        type="info"
        show-icon
        class="mb-3"
        message="No certificates yet."
        description="Click Issue certificate to request your first one. Configure ACME Settings (account email + directory URL) first if you haven't."
      />

      <Table
        :columns="columns"
        :data-source="items"
        :loading="loading"
        :pagination="{ pageSize: 25 }"
        row-key="id"
        size="middle"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'domain'">
            <Tooltip :title="record.cert_pem_path">
              <span class="domain">{{ record.domain }}</span>
            </Tooltip>
          </template>
          <template v-else-if="column.key === 'alt_names'">
            <Tag
              v-for="san in record.alt_names ?? []"
              :key="san"
              color="geekblue"
            >
              {{ san }}
            </Tag>
            <span
              v-if="!(record.alt_names && record.alt_names.length)"
              class="muted"
            >—</span>
          </template>
          <template v-else-if="column.key === 'challenge_type'">
            <Tag
              :color="record.challenge_type === 'dns-01' ? 'purple' : 'cyan'"
            >
              {{ record.challenge_type }}
            </Tag>
            <span v-if="record.dns_provider" class="muted">
              &nbsp;{{ record.dns_provider }}
            </span>
          </template>
          <template v-else-if="column.key === 'status'">
            <Tag :color="statusColor(record.status)">
              {{ record.status }}
            </Tag>
            <Tooltip
              v-if="record.last_error"
              :title="record.last_error"
              placement="topLeft"
            >
              <Tag color="red">!</Tag>
            </Tooltip>
          </template>
          <template v-else-if="column.key === 'expires_at'">
            <span v-if="record.expires_at" class="muted">
              {{ record.expires_at }}
            </span>
            <Tag
              v-if="expiryHint(record.expires_at)"
              :color="expiryHint(record.expires_at)?.color"
            >
              {{ expiryHint(record.expires_at)?.text }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Space>
              <Button
                size="small"
                :loading="renewingId === (record as AcmeCertificate).id"
                @click="renew((record as AcmeCertificate).id)"
              >
                Renew
              </Button>
              <Popconfirm
                :title="`Remove cert for ${record.domain}? PEM files on disk are NOT deleted (kumomta may still hold them open).`"
                ok-text="Remove"
                ok-type="danger"
                @confirm="removeRow((record as AcmeCertificate).id)"
              >
                <Button size="small" danger>Remove</Button>
              </Popconfirm>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="drawerOpen"
      title="Issue certificate"
      width="520"
      :destroy-on-close="true"
    >
      <Form :model="form" layout="vertical" :colon="false">
        <FormItem
          label="Domain (CN)"
          :rules="[{ required: true, message: 'Domain is required' }]"
        >
          <Input
            v-model:value="form.domain"
            placeholder="mta.example.com"
          />
          <div class="hint">
            Primary subject. The cert is keyed by this in the
            <code>acme_certificate</code> table; issuing the same domain twice
            updates the existing row.
          </div>
        </FormItem>

        <FormItem label="Subject Alt Names (optional)">
          <Select
            v-model:value="form.alt_names"
            mode="tags"
            placeholder="bounces.example.com, mail.example.com…"
            style="width: 100%"
            :token-separators="[',', ' ']"
          />
          <div class="hint">
            Add SANs to cover related hostnames on the same cert — typically
            your bounce subdomain so listener TLS works for both inbound
            mail <em>and</em> incoming DSNs.
          </div>
        </FormItem>

        <FormItem label="Challenge type">
          <RadioGroup v-model:value="form.challenge_type">
            <Radio value="http-01">HTTP-01</Radio>
            <Radio value="dns-01">DNS-01</Radio>
          </RadioGroup>
          <div v-if="form.challenge_type === 'http-01'" class="hint">
            The CA hits <code>http://&lt;domain&gt;/.well-known/acme-challenge/&lt;token&gt;</code>.
            iris's admin-service serves that path on
            <Tag color="geekblue">:80</Tag> automatically — the domain just
            has to resolve to this host's public IP.
          </div>
          <div v-else class="hint">
            The CA reads a TXT record from your DNS. Pick a configured DNS
            provider below; if the dropdown is empty, set one up on the
            <strong>DNS Providers</strong> page first.
          </div>
        </FormItem>

        <FormItem v-if="form.challenge_type === 'dns-01'" label="DNS provider">
          <Select
            v-model:value="form.dns_provider"
            placeholder="Pick a configured provider"
            style="width: 100%"
            :disabled="dnsProviders.length === 0"
          >
            <Select.Option
              v-for="p in dnsProvidersConfigured"
              :key="p"
              :value="p"
            >
              {{ p }}
            </Select.Option>
          </Select>
        </FormItem>
      </Form>

      <template #extra>
        <Space>
          <Button @click="drawerOpen = false">Cancel</Button>
          <Button type="primary" :loading="submitting" @click="submit">
            Issue
          </Button>
        </Space>
      </template>
    </Drawer>
  </Page>
</template>

<style scoped>
.domain {
  font-family: ui-monospace, 'SFMono-Regular', 'Menlo', 'Consolas', monospace;
  font-size: 13px;
}
.muted {
  color: var(--ant-color-text-tertiary);
  font-size: 12px;
}
.hint {
  font-size: 12px;
  color: var(--ant-color-text-tertiary);
  margin-top: 6px;
  line-height: 1.45;
}
.hint code {
  font-size: 12px;
}
</style>
