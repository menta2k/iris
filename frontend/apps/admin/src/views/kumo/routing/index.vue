<script lang="ts" setup>
import type {
  RoutingRule,
  RuleCondition,
  RuleTarget,
  Vmta,
  VmtaGroup,
} from '#/api/kumo';

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
  Select,
  SelectOption,
  Space,
  Switch,
  Table,
  Tag,
} from 'ant-design-vue';

import { routingApi, vmtaGroupsApi, vmtasApi } from '#/api/kumo';

defineOptions({ name: 'Routing' });

const items = ref<RoutingRule[]>([]);
const vmtas = ref<Vmta[]>([]);
const vmtaGroups = ref<VmtaGroup[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);

const FIELD_OPTIONS = [
  'from',
  'to',
  'to_domain',
  'from_domain',
  'header.subject',
  'source_ip',
];
const OP_OPTIONS = ['equals', 'contains', 'startswith', 'endswith', 'regex'];
const TARGET_KIND_OPTIONS: RuleTarget['kind'][] = [
  'vmta',
  'vmta_group',
  'queue',
  'reject',
  'discard',
];

interface FormState {
  name: string;
  priority: number;
  enabled: boolean;
  conditions: RuleCondition[];
  target: RuleTarget;
}

const form = reactive<FormState>(emptyForm());

function emptyForm(): FormState {
  return {
    name: '',
    priority: 100,
    enabled: true,
    conditions: [{ field: 'to_domain', op: 'equals', value: '' }],
    target: { kind: 'vmta', ref: '' },
  };
}

const columns = [
  {
    title: 'Priority',
    dataIndex: 'priority',
    key: 'priority',
    width: 90,
    sorter: (a: RoutingRule, b: RoutingRule) => a.priority - b.priority,
  },
  { title: 'Name', dataIndex: 'name', key: 'name', width: 220 },
  { title: 'Conditions', key: 'conditions' },
  { title: 'Target', key: 'target', width: 240 },
  { title: 'Enabled', dataIndex: 'enabled', key: 'enabled', width: 110 },
  { title: 'Actions', key: 'actions', width: 110 },
];

async function load() {
  loading.value = true;
  try {
    const [r, v, vg] = await Promise.all([
      routingApi.list(),
      vmtasApi.list().catch(() => ({ items: [] })),
      vmtaGroupsApi.list().catch(() => ({ items: [] })),
    ]);
    items.value = (r.items ?? [])
      .slice()
      .sort((a, b) => a.priority - b.priority);
    vmtas.value = v.items ?? [];
    vmtaGroups.value = vg.items ?? [];
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  Object.assign(form, emptyForm());
  drawerOpen.value = true;
}

function addCondition() {
  form.conditions.push({ field: 'to_domain', op: 'equals', value: '' });
}

function removeCondition(idx: number) {
  if (form.conditions.length > 1) form.conditions.splice(idx, 1);
}

async function submit() {
  if (!form.name.trim()) {
    message.warning('Name is required');
    return;
  }
  const cleanConditions = form.conditions.filter((c) => c.value.trim());
  if (cleanConditions.length === 0) {
    message.warning('At least one condition with a non-empty value is required');
    return;
  }
  const refRequired =
    form.target.kind === 'vmta' ||
    form.target.kind === 'vmta_group' ||
    form.target.kind === 'queue';
  if (refRequired && !form.target.ref?.trim()) {
    message.warning(`Pick a target ${form.target.kind} reference`);
    return;
  }
  submitting.value = true;
  try {
    await routingApi.create({
      name: form.name.trim(),
      priority: form.priority,
      enabled: form.enabled,
      conditions: cleanConditions,
      target: form.target,
    });
    message.success('Routing rule created');
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function toggle(rule: Record<string, any>, enabled: boolean) {
  await routingApi.update(rule.id, { enabled });
  rule.enabled = enabled;
}

async function removeRow(id: string) {
  await routingApi.remove(id);
  await load();
}

function targetSummary(t: RoutingRule['target']): string {
  if (t.kind === 'vmta' || t.kind === 'vmta_group' || t.kind === 'queue') {
    return `→ ${t.ref ?? '?'}`;
  }
  if (t.kind === 'reject') {
    return `${t.reject_code ?? ''} ${t.reject_text ?? ''}`.trim();
  }
  return '';
}

onMounted(load);
</script>

<template>
  <Page title="Routing Rules" description="First-match routing decisions for outbound mail">
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
          <template v-if="column.key === 'conditions'">
            <Space wrap>
              <Tag v-for="(c, i) in record.conditions" :key="i">
                {{ c.field }} {{ c.op }} {{ c.value }}
              </Tag>
              <Tag v-if="!record.conditions?.length">all</Tag>
            </Space>
          </template>
          <template v-else-if="column.key === 'target'">
            <Tag :color="record.target.kind === 'reject' ? 'red' : 'blue'">
              {{ record.target.kind }} {{ targetSummary(record.target) }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'enabled'">
            <Switch
              :checked="record.enabled"
              size="small"
              @change="(checked) => toggle(record, !!checked)"
            />
          </template>
          <template v-else-if="column.key === 'actions'">
            <Popconfirm
              title="Delete this routing rule?"
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
      title="New routing rule"
      width="640"
      :destroy-on-close="true"
    >
      <Form :model="form" layout="vertical">
        <FormItem
          label="Name"
          name="name"
          :rules="[{ required: true, message: 'Name is required' }]"
        >
          <Input v-model:value="form.name" placeholder="e.g. transactional-bulk" />
        </FormItem>

        <FormItem label="Priority" name="priority">
          <InputNumber v-model:value="form.priority" :min="0" :max="10_000" />
          <span style="margin-left: 8px; color: var(--ant-color-text-tertiary)">
            Lower numbers match first.
          </span>
        </FormItem>

        <FormItem label="Enabled" name="enabled">
          <Switch v-model:checked="form.enabled" />
        </FormItem>

        <FormItem label="Conditions (all must match)" name="conditions">
          <Space direction="vertical" style="width: 100%">
            <Space
              v-for="(c, idx) in form.conditions"
              :key="idx"
              style="width: 100%"
            >
              <Select v-model:value="c.field" style="width: 160px">
                <SelectOption v-for="f in FIELD_OPTIONS" :key="f" :value="f">
                  {{ f }}
                </SelectOption>
              </Select>
              <Select v-model:value="c.op" style="width: 130px">
                <SelectOption v-for="o in OP_OPTIONS" :key="o" :value="o">
                  {{ o }}
                </SelectOption>
              </Select>
              <Input v-model:value="c.value" placeholder="value" style="width: 240px" />
              <Button
                type="text"
                danger
                size="small"
                :disabled="form.conditions.length === 1"
                @click="removeCondition(idx)"
              >
                Remove
              </Button>
            </Space>
            <Button size="small" @click="addCondition">+ Add condition</Button>
          </Space>
        </FormItem>

        <FormItem label="Target kind" name="target.kind">
          <Select v-model:value="form.target.kind">
            <SelectOption v-for="k in TARGET_KIND_OPTIONS" :key="k" :value="k">
              {{ k }}
            </SelectOption>
          </Select>
        </FormItem>

        <FormItem
          v-if="form.target.kind === 'vmta'"
          label="Target VMTA"
          name="target.ref"
        >
          <Select
            v-model:value="form.target.ref"
            show-search
            placeholder="Pick a VMTA"
            style="width: 100%"
          >
            <SelectOption v-for="v in vmtas" :key="v.id" :value="v.name">
              {{ v.name }}
            </SelectOption>
          </Select>
        </FormItem>

        <FormItem
          v-else-if="form.target.kind === 'vmta_group'"
          label="Target VMTA group"
          name="target.ref"
        >
          <Select
            v-model:value="form.target.ref"
            show-search
            placeholder="Pick a VMTA group"
            style="width: 100%"
          >
            <SelectOption
              v-for="g in vmtaGroups"
              :key="g.id"
              :value="g.name"
              :disabled="!g.enabled"
            >
              {{ g.name }}{{ g.enabled ? '' : ' (disabled)' }}
            </SelectOption>
          </Select>
          <span style="color: var(--ant-color-text-tertiary)">
            Delivery is balanced across the group's members by weight.
          </span>
        </FormItem>

        <FormItem
          v-else-if="form.target.kind === 'queue'"
          label="Target queue name"
          name="target.ref"
        >
          <Input v-model:value="form.target.ref" placeholder="queue name" />
        </FormItem>

        <template v-if="form.target.kind === 'reject'">
          <FormItem label="Reject SMTP code" name="target.reject_code">
            <InputNumber
              v-model:value="form.target.reject_code"
              :min="400"
              :max="599"
              placeholder="550"
            />
          </FormItem>
          <FormItem label="Reject text" name="target.reject_text">
            <Input
              v-model:value="form.target.reject_text"
              placeholder="Recipient address suppressed"
            />
          </FormItem>
        </template>
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
