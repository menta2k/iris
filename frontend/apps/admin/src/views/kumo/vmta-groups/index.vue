<script lang="ts" setup>
import type { Vmta, VmtaGroup, VmtaGroupMember } from '#/api/kumo';

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
  Tooltip,
} from 'ant-design-vue';

import { vmtaGroupsApi, vmtasApi } from '#/api/kumo';

defineOptions({ name: 'VmtaGroups' });

const items = ref<VmtaGroup[]>([]);
const vmtas = ref<Vmta[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);
const editingId = ref<null | number>(null);

interface MemberRow extends VmtaGroupMember {
  // local-only key for v-for stability when the user adds rows
  uid: string;
}

const form = reactive<{
  name: string;
  description: string;
  enabled: boolean;
  members: MemberRow[];
}>({
  name: '',
  description: '',
  enabled: true,
  members: [],
});

const columns = [
  { title: 'Name', dataIndex: 'name', key: 'name', width: 220 },
  { title: 'Description', dataIndex: 'description', key: 'description' },
  { title: 'Enabled', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: 'Members', key: 'members_summary' },
  {
    title: 'Total weight',
    key: 'total_weight',
    width: 120,
    align: 'right' as const,
  },
  { title: 'Actions', key: 'actions', width: 200 },
];

function totalWeight(g: Record<string, any>): number {
  const members = (g.members ?? []) as VmtaGroupMember[];
  return members
    .filter((m) => m.enabled && m.weight > 0)
    .reduce((sum, m) => sum + m.weight, 0);
}

async function load() {
  loading.value = true;
  try {
    const [g, v] = await Promise.all([
      vmtaGroupsApi.list(),
      vmtasApi.list().catch(() => ({ items: [] })),
    ]);
    // List endpoint returns groups without members; fetch each detail in
    // parallel so the row counts are accurate. Acceptable for small N.
    const detailed = await Promise.all(
      (g.items ?? []).map((it) => vmtaGroupsApi.get(it.id)),
    );
    items.value = detailed;
    vmtas.value = v.items ?? [];
  } catch {
    items.value = [];
  } finally {
    loading.value = false;
  }
}

let uidCounter = 0;
function newUid() {
  uidCounter += 1;
  return `m${uidCounter}`;
}

function emptyMemberRow(): MemberRow {
  return {
    uid: newUid(),
    vmta_id: 0,
    vmta_name: '',
    weight: 1,
    priority: 0,
    enabled: true,
  };
}

function openCreate() {
  editingId.value = null;
  form.name = '';
  form.description = '';
  form.enabled = true;
  form.members = [emptyMemberRow()];
  drawerOpen.value = true;
}

function openEdit(item: Record<string, any>) {
  const g = item as VmtaGroup;
  editingId.value = g.id;
  form.name = g.name;
  form.description = g.description ?? '';
  form.enabled = g.enabled;
  form.members = (g.members ?? []).map((m) => ({ ...m, uid: newUid() }));
  if (form.members.length === 0) form.members.push(emptyMemberRow());
  drawerOpen.value = true;
}

function addMember() {
  form.members.push(emptyMemberRow());
}

function removeMember(uid: string) {
  if (form.members.length > 1) {
    form.members = form.members.filter((m) => m.uid !== uid);
  }
}

async function submit() {
  if (!form.name.trim()) {
    message.warning('Name is required');
    return;
  }
  const cleanMembers = form.members.filter((m) => m.vmta_id > 0);
  if (cleanMembers.length === 0) {
    message.warning('At least one member VMTA is required');
    return;
  }
  // Detect dupes early so the backend doesn't have to surface a constraint err.
  const seen = new Set<number>();
  for (const m of cleanMembers) {
    if (seen.has(m.vmta_id)) {
      message.warning(`VMTA "${m.vmta_name}" appears twice — remove the duplicate`);
      return;
    }
    seen.add(m.vmta_id);
  }
  submitting.value = true;
  try {
    let groupId = editingId.value;
    const payload = {
      name: form.name.trim(),
      description: form.description || undefined,
      enabled: form.enabled,
    };
    if (groupId === null) {
      const created = await vmtaGroupsApi.create(payload);
      groupId = created.id;
    } else {
      await vmtaGroupsApi.update(groupId, payload);
    }
    await vmtaGroupsApi.setMembers(
      groupId,
      cleanMembers.map((m) => ({
        vmta_id: m.vmta_id,
        weight: m.weight,
        priority: m.priority,
        enabled: m.enabled,
      })),
    );
    message.success(
      editingId.value === null ? 'Group created' : 'Group updated',
    );
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function removeRow(id: number) {
  await vmtaGroupsApi.remove(id);
  await load();
}

function onMemberVmtaChange(member: MemberRow, vmtaId: number) {
  const v = vmtas.value.find((x) => Number(x.id) === Number(vmtaId));
  member.vmta_id = vmtaId;
  member.vmta_name = v?.name ?? '';
}

function memberSharePct(
  member: VmtaGroupMember,
  group: Record<string, any>,
): string {
  const total = totalWeight(group);
  if (!member.enabled || member.weight === 0 || total === 0) return '0%';
  return `${((member.weight / total) * 100).toFixed(1)}%`;
}

onMounted(load);
</script>

<template>
  <Page
    title="VMTA Groups"
    description="Bundles of Virtual MTAs with weighted-random load balancing"
  >
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">New group</Button>
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

          <template v-else-if="column.key === 'members_summary'">
            <Space wrap>
              <Tag
                v-for="m in record.members ?? []"
                :key="m.vmta_id"
                :color="m.enabled && m.weight > 0 ? 'blue' : 'default'"
              >
                {{ m.vmta_name || `vmta#${m.vmta_id}` }} ·
                w={{ m.weight }}{{ m.priority > 0 ? ` p=${m.priority}` : '' }}
                ({{ memberSharePct(m, record) }})
              </Tag>
              <Tag v-if="!record.members?.length">empty</Tag>
            </Space>
          </template>

          <template v-else-if="column.key === 'total_weight'">
            {{ totalWeight(record) }}
          </template>

          <template v-else-if="column.key === 'actions'">
            <Space>
              <Button size="small" @click="openEdit(record)">Edit</Button>
              <Popconfirm
                title="Delete this group? Routing rules referencing it by name will start failing validation."
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
      :title="editingId === null ? 'New VMTA group' : 'Edit VMTA group'"
      width="720"
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
            placeholder="e.g. eu-bulk"
          />
        </FormItem>

        <FormItem label="Description" name="description">
          <Input.TextArea v-model:value="form.description" :rows="2" />
        </FormItem>

        <FormItem label="Enabled" name="enabled">
          <Switch v-model:checked="form.enabled" />
          <span style="margin-left: 8px; color: var(--ant-color-text-tertiary)">
            Disabling a group hides it from the policy renderer (existing
            routing rules referencing it will fail validation until edited).
          </span>
        </FormItem>

        <FormItem label="Members" name="members">
          <Space direction="vertical" style="width: 100%">
            <Space
              v-for="m in form.members"
              :key="m.uid"
              wrap
              style="width: 100%"
            >
              <Select
                :value="m.vmta_id"
                show-search
                placeholder="Pick a VMTA"
                style="width: 240px"
                option-filter-prop="label"
                @change="(val) => onMemberVmtaChange(m, Number(val))"
              >
                <SelectOption
                  v-for="v in vmtas"
                  :key="v.id"
                  :value="Number(v.id)"
                  :label="v.name"
                >
                  {{ v.name }}
                </SelectOption>
              </Select>

              <Tooltip title="Higher weight = larger share of traffic">
                <InputNumber
                  v-model:value="m.weight"
                  :min="0"
                  :max="10_000"
                  addon-before="weight"
                  style="width: 130px"
                />
              </Tooltip>

              <Tooltip title="Lower priority tier is tried first">
                <InputNumber
                  v-model:value="m.priority"
                  :min="0"
                  :max="100"
                  addon-before="priority"
                  style="width: 140px"
                />
              </Tooltip>

              <Switch
                v-model:checked="m.enabled"
                checked-children="enabled"
                un-checked-children="disabled"
              />

              <Button
                type="text"
                danger
                size="small"
                :disabled="form.members.length === 1"
                @click="removeMember(m.uid)"
              >
                Remove
              </Button>
            </Space>
            <Button size="small" @click="addMember">+ Add member</Button>
          </Space>
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
