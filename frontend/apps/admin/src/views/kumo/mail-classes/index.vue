<script lang="ts" setup>
import type { MailClass, Vmta, VmtaGroup } from '#/api/kumo';

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
  SelectOption,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'ant-design-vue';

import { mailClassesApi, vmtaGroupsApi, vmtasApi } from '#/api/kumo';

defineOptions({ name: 'MailClasses' });

// The header name kumomta inspects at message reception. Currently a global
// constant matched by the backend renderer; surfaced here so operators can
// see what to send in their HTTP/SMTP integration.
const MAIL_CLASS_HEADER = 'X-Kumo-Mail-Class';

const items = ref<MailClass[]>([]);
const vmtas = ref<Vmta[]>([]);
const vmtaGroups = ref<VmtaGroup[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);
const editingId = ref<null | number>(null);

const form = reactive<{
  name: string;
  description: string;
  enabled: boolean;
  target_kind: 'vmta' | 'vmta_group';
  target_ref: string;
}>({
  name: '',
  description: '',
  enabled: true,
  target_kind: 'vmta',
  target_ref: '',
});

const targetOptions = computed(() => {
  if (form.target_kind === 'vmta_group') {
    return vmtaGroups.value.map((g) => ({
      value: g.name,
      label: g.enabled ? g.name : `${g.name} (disabled)`,
      disabled: !g.enabled,
    }));
  }
  return vmtas.value.map((v) => ({ value: v.name, label: v.name }));
});

const columns = [
  { title: 'Name', dataIndex: 'name', key: 'name', width: 200 },
  { title: 'Description', dataIndex: 'description', key: 'description' },
  { title: 'Enabled', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: 'Target', key: 'target', width: 280 },
  { title: 'Actions', key: 'actions', width: 160 },
];

async function load() {
  loading.value = true;
  try {
    const [mc, v, vg] = await Promise.all([
      mailClassesApi.list(),
      vmtasApi.list().catch(() => ({ items: [] })),
      vmtaGroupsApi.list().catch(() => ({ items: [] })),
    ]);
    items.value = mc.items ?? [];
    vmtas.value = v.items ?? [];
    vmtaGroups.value = vg.items ?? [];
  } catch {
    items.value = [];
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editingId.value = null;
  form.name = '';
  form.description = '';
  form.enabled = true;
  form.target_kind = 'vmta';
  form.target_ref = '';
  drawerOpen.value = true;
}

function openEdit(item: Record<string, any>) {
  const mc = item as MailClass;
  editingId.value = mc.id;
  form.name = mc.name;
  form.description = mc.description ?? '';
  form.enabled = mc.enabled;
  form.target_kind = mc.target_kind;
  form.target_ref = mc.target_ref ?? '';
  drawerOpen.value = true;
}

function onTargetKindChange() {
  // Clear the ref so a stale VMTA name doesn't survive into a vmta_group pick.
  form.target_ref = '';
}

async function submit() {
  if (!form.name.trim()) {
    message.warning('Name is required');
    return;
  }
  if (!form.target_ref) {
    message.warning('Pick a target VMTA or VMTA group');
    return;
  }
  submitting.value = true;
  try {
    const payload = {
      name: form.name.trim(),
      description: form.description || undefined,
      enabled: form.enabled,
      target_kind: form.target_kind,
      target_ref: form.target_ref,
    };
    if (editingId.value === null) {
      await mailClassesApi.create(payload);
      message.success('Mail class created');
    } else {
      await mailClassesApi.update(editingId.value, payload);
      message.success('Mail class updated');
    }
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function removeRow(id: number) {
  await mailClassesApi.remove(id);
  await load();
}

function targetExists(record: Record<string, any>): boolean {
  const r = record as MailClass;
  if (r.target_kind === 'vmta_group') {
    return vmtaGroups.value.some((g) => g.name === r.target_ref);
  }
  return vmtas.value.some((v) => v.name === r.target_ref);
}

onMounted(load);
</script>

<template>
  <Page
    title="Mail Classes"
    description="Header-driven routing: messages with X-Kumo-Mail-Class set to a class name are routed to that class's target."
  >
    <Alert
      type="info"
      show-icon
      class="mb-3"
      :message="`Senders set the header ${MAIL_CLASS_HEADER}: <class-name> via HTTP or SMTP. Matching messages skip routing rules and go straight to the configured VMTA or VMTA group.`"
    />

    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">New mail class</Button>
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

          <template v-else-if="column.key === 'target'">
            <Space wrap>
              <Tag :color="record.target_kind === 'vmta_group' ? 'purple' : 'blue'">
                {{ record.target_kind }}
              </Tag>
              <Typography.Text>
                {{ record.target_ref || '—' }}
              </Typography.Text>
              <Tag v-if="!targetExists(record)" color="orange">
                target missing
              </Tag>
            </Space>
          </template>

          <template v-else-if="column.key === 'actions'">
            <Space>
              <Button size="small" @click="openEdit(record)">Edit</Button>
              <Popconfirm
                title="Delete this mail class?"
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
      :title="editingId === null ? 'New mail class' : 'Edit mail class'"
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
            :disabled="editingId !== null"
            placeholder="e.g. transactional"
          />
          <span style="color: var(--ant-color-text-tertiary)">
            Senders write this exact value into the
            <code>{{ MAIL_CLASS_HEADER }}</code> header.
          </span>
        </FormItem>

        <FormItem label="Description" name="description">
          <Input.TextArea v-model:value="form.description" :rows="2" />
        </FormItem>

        <FormItem label="Enabled" name="enabled">
          <Switch v-model:checked="form.enabled" />
          <span style="margin-left: 8px; color: var(--ant-color-text-tertiary)">
            Disabled classes are skipped by the renderer.
          </span>
        </FormItem>

        <FormItem
          label="Target kind"
          name="target_kind"
          :rules="[{ required: true, message: 'Required' }]"
        >
          <Select v-model:value="form.target_kind" @change="onTargetKindChange">
            <SelectOption value="vmta">VMTA</SelectOption>
            <SelectOption value="vmta_group">VMTA group</SelectOption>
          </Select>
        </FormItem>

        <FormItem
          :label="form.target_kind === 'vmta_group' ? 'Target VMTA group' : 'Target VMTA'"
          name="target_ref"
          :rules="[{ required: true, message: 'Pick a target' }]"
        >
          <Select
            v-model:value="form.target_ref"
            show-search
            placeholder="Pick a target by name"
            style="width: 100%"
            :options="targetOptions"
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
