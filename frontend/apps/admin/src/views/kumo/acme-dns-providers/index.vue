<script lang="ts" setup>
import type {
  AcmeDnsProviderConfig,
  AcmeProviderInfo,
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
  Select,
  Space,
  Table,
  Tag,
} from 'ant-design-vue';

import { acmeApi } from '#/api/kumo';

defineOptions({ name: 'AcmeDnsProviders' });

const loading = ref(false);
const submitting = ref(false);
const items = ref<AcmeDnsProviderConfig[]>([]);
const registry = ref<AcmeProviderInfo[]>([]);

const drawerOpen = ref(false);
const editing = ref<null | AcmeDnsProviderConfig>(null);
// Which credential fields already have a stored value (from the API's
// configured_keys). The API never returns the secret values themselves, so
// on edit these fields stay blank and "leave blank to keep" applies.
const editingConfigured = ref<Set<string>>(new Set());

// The drawer uses a single reactive form whose shape changes with the
// selected provider. We keep all values as strings (the API expects
// map<string, string>) and let the operator type whatever the
// registry's required/optional fields say.
const form = reactive<{ provider: string; values: Record<string, string> }>({
  provider: '',
  values: {},
});

// Lookup helper. Returns the registry metadata (description + field
// list) for the currently-selected provider in the drawer, or null.
const selectedInfo = computed<AcmeProviderInfo | null>(() => {
  if (!form.provider) return null;
  return registry.value.find((p) => p.name === form.provider) ?? null;
});

// Set of provider names already configured — used to dim them in the
// "new" picker so the operator can't try to create a second config for
// the same provider (the backend would 409 anyway).
const configuredSet = computed(
  () => new Set(items.value.map((c) => c.provider)),
);

const columns = [
  { title: 'Provider', dataIndex: 'provider', key: 'provider', width: 200 },
  {
    title: 'Description',
    dataIndex: '_desc',
    key: '_desc',
    ellipsis: true,
  },
  { title: 'Updated', dataIndex: 'updated_at', key: 'updated_at', width: 200 },
  { title: 'Actions', key: 'actions', width: 200 },
];

async function load() {
  loading.value = true;
  try {
    const [reg, configs] = await Promise.all([
      acmeApi.listRegistry(),
      acmeApi.listDnsProviderConfigs(),
    ]);
    registry.value = (reg.items ?? []).slice().sort((a, b) =>
      a.name.localeCompare(b.name),
    );
    items.value = configs.items ?? [];
  } finally {
    loading.value = false;
  }
}

function descriptionOf(name: string): string {
  return registry.value.find((p) => p.name === name)?.description ?? '';
}

function openCreate() {
  editing.value = null;
  editingConfigured.value = new Set();
  form.provider = '';
  form.values = {};
  drawerOpen.value = true;
}

function openEdit(row: AcmeDnsProviderConfig) {
  editing.value = row;
  form.provider = row.provider;
  editingConfigured.value = new Set(row.configured_keys ?? []);
  // Secrets are never returned by the API, so start every field blank.
  // The operator fills only what they want to change; blanks keep the
  // stored value (the backend merges). Seed keys from the registry schema
  // so the v-model bindings exist.
  const info = registry.value.find((p) => p.name === row.provider);
  const next: Record<string, string> = {};
  for (const f of info?.required_fields ?? []) next[f] = '';
  for (const f of info?.optional_fields ?? []) next[f] = '';
  form.values = next;
  drawerOpen.value = true;
}

// True when a field already holds a stored value (on edit). Used to relax
// required-field validation and to show a "saved" placeholder.
function isConfigured(field: string): boolean {
  return editingConfigured.value.has(field);
}

function fieldPlaceholder(field: string): string {
  return isConfigured(field) ? '•••••• saved — leave blank to keep' : field;
}

// Triggered when the operator picks a provider in the "new" flow.
// Pre-populates the values map with empty strings for every required
// + optional field so the v-model bindings have keys to attach to.
function onProviderChange(name: string) {
  form.provider = name;
  const info = registry.value.find((p) => p.name === name);
  if (!info) return;
  const next: Record<string, string> = {};
  for (const f of info.required_fields ?? []) next[f] = form.values[f] ?? '';
  for (const f of info.optional_fields ?? []) next[f] = form.values[f] ?? '';
  form.values = next;
}

async function save() {
  if (!form.provider) {
    message.warning('Pick a provider');
    return;
  }
  // Required-field check up front — the backend would also reject but
  // a client-side error is faster.
  const info = selectedInfo.value;
  if (info) {
    // A required field may be left blank only if it already has a stored
    // value (edit + keep). New providers must fill every required field.
    const missing = (info.required_fields ?? []).filter(
      (f) => !(form.values[f] ?? '').trim() && !isConfigured(f),
    );
    if (missing.length > 0) {
      message.warning(`Missing required field(s): ${missing.join(', ')}`);
      return;
    }
  }
  submitting.value = true;
  try {
    // Drop empty optional fields so we don't store noise.
    const payload: Record<string, string> = {};
    for (const [k, v] of Object.entries(form.values)) {
      if (v != null && String(v).trim() !== '') payload[k] = String(v).trim();
    }
    await acmeApi.saveDnsProviderConfig({
      provider: form.provider,
      config: payload,
    });
    message.success(editing.value ? 'Provider updated' : 'Provider saved');
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function removeRow(provider: string) {
  await acmeApi.removeDnsProviderConfig(provider);
  message.success(`Removed ${provider}`);
  await load();
}

// Heuristic for which fields to mask in the form. Anything that looks
// like a credential gets a password-style input; everything else is
// plain. The list is sloppy on purpose — better to mask too much
// than too little.
const SECRETY = /token|password|key|secret|credential/i;
function isSecretField(name: string): boolean {
  return SECRETY.test(name);
}

onMounted(load);
</script>

<template>
  <Page
    title="DNS Providers"
    description="Saved credentials for DNS-01 challenge providers. The list of supported providers and their fields comes from the backend registry, so adding a new provider only takes a backend change."
  >
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">Add provider</Button>
        <Button :loading="loading" @click="load">Refresh</Button>
      </Space>

      <Alert
        v-if="!loading && items.length === 0"
        type="info"
        show-icon
        class="mb-3"
        message="No DNS providers configured yet."
        description="Add a provider here to enable DNS-01 issuance. HTTP-01 issuance doesn't need any provider config — it uses the iris admin-service :80 listener."
      />

      <Table
        :columns="columns"
        :data-source="items"
        :loading="loading"
        :pagination="{ pageSize: 25 }"
        row-key="provider"
        size="middle"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'provider'">
            <Tag color="geekblue">{{ record.provider }}</Tag>
          </template>
          <template v-else-if="column.key === '_desc'">
            <span class="muted">{{ descriptionOf(record.provider) }}</span>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Space>
              <Button size="small" @click="openEdit(record as AcmeDnsProviderConfig)">
                Edit
              </Button>
              <Popconfirm
                :title="`Remove ${record.provider} credentials?`"
                ok-text="Remove"
                ok-type="danger"
                @confirm="removeRow((record as AcmeDnsProviderConfig).provider)"
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
      :title="editing ? `Edit ${editing.provider}` : 'Add DNS provider'"
      width="540"
      :destroy-on-close="true"
    >
      <Form layout="vertical" :colon="false">
        <FormItem
          label="Provider"
          :rules="[{ required: true, message: 'Pick a provider' }]"
        >
          <Select
            :value="form.provider"
            placeholder="Pick a provider…"
            :disabled="!!editing"
            style="width: 100%"
            @change="(v) => onProviderChange(v as string)"
          >
            <Select.Option
              v-for="p in registry"
              :key="p.name"
              :value="p.name"
              :disabled="!editing && configuredSet.has(p.name)"
            >
              {{ p.name }}
              <span class="opt-desc">— {{ p.description }}</span>
            </Select.Option>
          </Select>
        </FormItem>

        <template v-if="selectedInfo">
          <Alert
            type="info"
            show-icon
            class="mb-3"
            :message="selectedInfo.description"
          />

          <div class="section-label">Required</div>
          <FormItem
            v-for="field in selectedInfo.required_fields"
            :key="`req-${field}`"
            :label="field"
            :required="!isConfigured(field)"
          >
            <Input.Password
              v-if="isSecretField(field)"
              v-model:value="form.values[field]"
              :placeholder="fieldPlaceholder(field)"
            />
            <Input
              v-else
              v-model:value="form.values[field]"
              :placeholder="fieldPlaceholder(field)"
            />
          </FormItem>

          <template v-if="selectedInfo.optional_fields?.length">
            <div class="section-label">Optional</div>
            <FormItem
              v-for="field in selectedInfo.optional_fields"
              :key="`opt-${field}`"
              :label="field"
            >
              <Input.Password
                v-if="isSecretField(field)"
                v-model:value="form.values[field]"
                :placeholder="fieldPlaceholder(field)"
              />
              <Input
                v-else
                v-model:value="form.values[field]"
                :placeholder="fieldPlaceholder(field)"
              />
            </FormItem>
          </template>
        </template>
      </Form>

      <template #extra>
        <Space>
          <Button @click="drawerOpen = false">Cancel</Button>
          <Button type="primary" :loading="submitting" @click="save">
            Save
          </Button>
        </Space>
      </template>
    </Drawer>
  </Page>
</template>

<style scoped>
.muted {
  color: var(--ant-color-text-tertiary);
  font-size: 12px;
}
.opt-desc {
  color: var(--ant-color-text-tertiary);
  font-size: 12px;
}
.section-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--ant-color-text-tertiary);
  margin: 16px 0 8px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
</style>
