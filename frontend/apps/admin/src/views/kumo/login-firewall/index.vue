<script lang="ts" setup>
import type {
  LoginPolicy,
  LoginPolicyInput,
  LoginPolicyMethod,
  LoginPolicyType,
  User,
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
  Modal,
  Popconfirm,
  Select,
  SelectOption,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'ant-design-vue';

import { loginPoliciesApi, usersApi } from '#/api/kumo';

defineOptions({ name: 'LoginFirewall' });

// Go time.Weekday ordering (0 = Sunday) so values round-trip to the backend
// time_window.days field without translation.
const WEEKDAYS = [
  { value: 1, label: 'Mon' },
  { value: 2, label: 'Tue' },
  { value: 3, label: 'Wed' },
  { value: 4, label: 'Thu' },
  { value: 5, label: 'Fri' },
  { value: 6, label: 'Sat' },
  { value: 0, label: 'Sun' },
];

const items = ref<LoginPolicy[]>([]);
const users = ref<User[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);
const lockoutModalOpen = ref(false);
const editingId = ref<null | number>(null);

const form = reactive<{
  days: number[];
  enabled: boolean;
  end: string;
  method: LoginPolicyMethod;
  reason: string;
  start: string;
  targetId: number;
  timezone: string;
  type: LoginPolicyType;
  value: string;
}>({
  days: [],
  enabled: true,
  end: '',
  method: 'IP',
  reason: '',
  start: '',
  targetId: 0,
  timezone: '',
  type: 'BLACKLIST',
  value: '',
});

const userOptions = computed(() => [
  { value: 0, label: 'Global — all users' },
  ...users.value.map((u) => ({ value: u.id, label: `${u.username} (#${u.id})` })),
]);

const columns = [
  { title: 'Scope', key: 'scope', width: 180 },
  { title: 'Type', dataIndex: 'type', key: 'type', width: 120 },
  { title: 'Method', dataIndex: 'method', key: 'method', width: 110 },
  { title: 'Match', key: 'match' },
  { title: 'Enabled', dataIndex: 'enabled', key: 'enabled', width: 90 },
  { title: 'Reason', dataIndex: 'reason', key: 'reason' },
  { title: 'Actions', key: 'actions', width: 160 },
];

function usernameFor(id?: number): string {
  if (!id) return 'Global';
  const u = users.value.find((x) => x.id === id);
  return u ? u.username : `User #${id}`;
}

function matchSummary(input: Record<string, any>): string {
  const record = input as LoginPolicy;
  if (record.method === 'TIME') {
    const tw = record.timeWindow;
    if (!tw) return '—';
    const days =
      tw.days && tw.days.length > 0
        ? tw.days
            .map((d) => WEEKDAYS.find((w) => w.value === d)?.label ?? d)
            .join(',')
        : 'every day';
    const tz = tw.timezone || 'UTC';
    return `${days} ${tw.start}–${tw.end} (${tz})`;
  }
  return record.value || '—';
}

async function load() {
  loading.value = true;
  try {
    const [policies, u] = await Promise.all([
      loginPoliciesApi.list(),
      usersApi.list().catch(() => ({ items: [] as User[] })),
    ]);
    items.value = policies.items ?? [];
    users.value = u.items ?? [];
  } catch {
    items.value = [];
  } finally {
    loading.value = false;
  }
}

function resetForm() {
  form.days = [];
  form.enabled = true;
  form.end = '';
  form.method = 'IP';
  form.reason = '';
  form.start = '';
  form.targetId = 0;
  form.timezone = '';
  form.type = 'BLACKLIST';
  form.value = '';
}

function openCreate() {
  editingId.value = null;
  resetForm();
  drawerOpen.value = true;
}

function openEdit(record: Record<string, any>) {
  const p = record as LoginPolicy;
  editingId.value = p.id;
  resetForm();
  form.targetId = p.targetId ?? 0;
  form.type = p.type;
  form.method = (p.method as LoginPolicyMethod) ?? 'IP';
  form.reason = p.reason ?? '';
  form.enabled = p.enabled ?? true;
  if (p.method === 'TIME' && p.timeWindow) {
    form.days = [...p.timeWindow.days];
    form.start = p.timeWindow.start;
    form.end = p.timeWindow.end;
    form.timezone = p.timeWindow.timezone;
  } else {
    form.value = p.value ?? '';
  }
  drawerOpen.value = true;
}

function buildPayload(): LoginPolicyInput {
  const payload: LoginPolicyInput = {
    targetId: form.targetId || 0,
    type: form.type,
    method: form.method,
    reason: form.reason.trim() || undefined,
    enabled: form.enabled,
  };
  if (form.method === 'TIME') {
    payload.timeWindow = {
      days: [...form.days],
      start: form.start.trim(),
      end: form.end.trim(),
      timezone: form.timezone.trim(),
    };
  } else {
    payload.value = form.value.trim();
  }
  return payload;
}

function validate(): boolean {
  if (form.method === 'TIME') {
    if (!form.start.trim() || !form.end.trim()) {
      message.warning('Start and end time are required (HH:MM)');
      return false;
    }
  } else if (!form.value.trim()) {
    message.warning(
      form.method === 'IP' ? 'IP or CIDR is required' : 'Country code is required',
    );
    return false;
  }
  return true;
}

function isLockoutError(error: unknown): boolean {
  const e = error as { response?: { data?: { code?: string }; status?: number } };
  return (
    e?.response?.status === 409 &&
    e?.response?.data?.code === 'WOULD_LOCK_OUT_SELF'
  );
}

async function save(acknowledge: boolean) {
  submitting.value = true;
  try {
    const payload = buildPayload();
    if (editingId.value === null) {
      await loginPoliciesApi.create(payload, acknowledge);
      message.success('Rule created');
    } else {
      await loginPoliciesApi.update(editingId.value, payload, acknowledge);
      message.success('Rule updated');
    }
    drawerOpen.value = false;
    lockoutModalOpen.value = false;
    await load();
  } catch (error: unknown) {
    // The backend's self-lockout guard returns 409; offer an explicit
    // override instead of the generic error toast.
    if (isLockoutError(error)) {
      lockoutModalOpen.value = true;
    }
    // Any other error already surfaced a toast via the request interceptor.
  } finally {
    submitting.value = false;
  }
}

function submit() {
  if (!validate()) return;
  void save(false);
}

async function removeRow(id: number) {
  await loginPoliciesApi.remove(id);
  await load();
}

onMounted(load);
</script>

<template>
  <Page
    title="Login Firewall"
    description="Gate who can authenticate by IP/CIDR, country, or time window. Rules apply globally or to a single user."
  >
    <Alert
      type="info"
      show-icon
      class="mb-3"
      message="Blacklist rules deny a login when they match. Whitelist rules restrict a method to its listed values (a method with no whitelist rule is unrestricted). When an attribute can't be determined — missing client IP, GeoIP database absent — that method's rules are skipped (fail-open) to avoid accidental lockouts."
    />

    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">New rule</Button>
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
          <template v-if="column.key === 'scope'">
            <Tag :color="record.targetId ? 'blue' : 'default'">
              {{ usernameFor(record.targetId) }}
            </Tag>
          </template>

          <template v-else-if="column.key === 'type'">
            <Tag :color="record.type === 'BLACKLIST' ? 'red' : 'green'">
              {{ record.type }}
            </Tag>
          </template>

          <template v-else-if="column.key === 'method'">
            <Tag color="geekblue">{{ record.method }}</Tag>
          </template>

          <template v-else-if="column.key === 'match'">
            <Typography.Text code>{{ matchSummary(record) }}</Typography.Text>
          </template>

          <template v-else-if="column.key === 'enabled'">
            <Tag :color="record.enabled ? 'green' : 'default'">
              {{ record.enabled ? 'Yes' : 'No' }}
            </Tag>
          </template>

          <template v-else-if="column.key === 'reason'">
            <Typography.Text type="secondary">
              {{ record.reason || '—' }}
            </Typography.Text>
          </template>

          <template v-else-if="column.key === 'actions'">
            <Space>
              <Button size="small" @click="openEdit(record)">Edit</Button>
              <Popconfirm
                title="Delete this rule?"
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
      :title="editingId === null ? 'New login rule' : 'Edit login rule'"
      width="520"
      :destroy-on-close="true"
    >
      <Form :model="form" layout="vertical">
        <FormItem label="Applies to">
          <Select
            v-model:value="form.targetId"
            show-search
            option-filter-prop="label"
            :options="userOptions"
          />
          <span style="color: var(--ant-color-text-tertiary)">
            Global rules are evaluated for every login.
          </span>
        </FormItem>

        <FormItem label="Type" name="type">
          <Select v-model:value="form.type">
            <SelectOption value="BLACKLIST">Blacklist — deny on match</SelectOption>
            <SelectOption value="WHITELIST">
              Whitelist — allow only matching
            </SelectOption>
          </Select>
        </FormItem>

        <FormItem label="Method" name="method">
          <Select v-model:value="form.method">
            <SelectOption value="IP">IP / CIDR</SelectOption>
            <SelectOption value="REGION">Country (region)</SelectOption>
            <SelectOption value="TIME">Time window</SelectOption>
          </Select>
        </FormItem>

        <FormItem v-if="form.method === 'IP'" label="IP or CIDR" name="value">
          <Input v-model:value="form.value" placeholder="e.g. 203.0.113.5 or 10.0.0.0/8" />
        </FormItem>

        <FormItem
          v-else-if="form.method === 'REGION'"
          label="Country code"
          name="value"
        >
          <Input
            v-model:value="form.value"
            placeholder="ISO 3166-1 alpha-2, e.g. BG"
            :maxlength="2"
          />
          <span style="color: var(--ant-color-text-tertiary)">
            Requires a GeoIP database on the server; otherwise region rules are skipped.
          </span>
        </FormItem>

        <template v-else-if="form.method === 'TIME'">
          <FormItem label="Weekdays">
            <Select
              v-model:value="form.days"
              mode="multiple"
              placeholder="Empty = every day"
              :options="WEEKDAYS"
            />
          </FormItem>
          <Space>
            <FormItem label="Start (HH:MM)" name="start">
              <Input v-model:value="form.start" placeholder="09:00" style="width: 140px" />
            </FormItem>
            <FormItem label="End (HH:MM)" name="end">
              <Input v-model:value="form.end" placeholder="17:00" style="width: 140px" />
            </FormItem>
          </Space>
          <FormItem label="Timezone">
            <Input v-model:value="form.timezone" placeholder="IANA, e.g. Europe/Sofia (empty = UTC)" />
          </FormItem>
        </template>

        <FormItem label="Enabled" name="enabled">
          <Switch v-model:checked="form.enabled" />
          <span style="margin-left: 8px; color: var(--ant-color-text-tertiary)">
            Disabled rules are kept but not enforced.
          </span>
        </FormItem>

        <FormItem label="Reason" name="reason">
          <Input.TextArea
            v-model:value="form.reason"
            :rows="2"
            placeholder="Optional — shown in the audit trail"
          />
        </FormItem>
      </Form>
      <template #extra>
        <Space>
          <Button @click="drawerOpen = false">Cancel</Button>
          <Button type="primary" :loading="submitting" @click="submit">Save</Button>
        </Space>
      </template>
    </Drawer>

    <Modal
      v-model:open="lockoutModalOpen"
      title="This rule would block your own login"
      ok-text="Apply anyway"
      ok-type="danger"
      :confirm-loading="submitting"
      @ok="save(true)"
      @cancel="lockoutModalOpen = false"
    >
      <p>
        Based on your current IP / location / time, this rule would prevent you
        from logging in. Apply it anyway?
      </p>
    </Modal>
  </Page>
</template>
