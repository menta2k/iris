<script lang="ts" setup>
import type { User } from '#/api/kumo';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useUserStore } from '@vben/stores';

import {
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

import { usersApi } from '#/api/kumo';

defineOptions({ name: 'Users' });

const userStore = useUserStore();
const currentUserId = computed<null | number>(() => {
  const id = userStore.userInfo?.id;
  return typeof id === 'number' ? id : null;
});

const items = ref<User[]>([]);
const loading = ref(false);
const drawerOpen = ref(false);
const submitting = ref(false);

const passwordDrawerOpen = ref(false);
const passwordSubmitting = ref(false);
const passwordTarget = ref<null | User>(null);
const passwordForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
});

const passwordIsSelf = computed(
  () =>
    passwordTarget.value !== null &&
    currentUserId.value !== null &&
    passwordTarget.value.id === currentUserId.value,
);

const form = reactive({
  username: '',
  email: '',
  password: '',
  roles: ['operator'] as string[],
});

const ROLE_OPTIONS = [
  { value: 'admin', label: 'admin' },
  { value: 'operator', label: 'operator' },
  { value: 'viewer', label: 'viewer' },
];

const columns = [
  { title: 'Username', dataIndex: 'username', key: 'username', width: 180 },
  { title: 'Email', dataIndex: 'email', key: 'email' },
  { title: 'Roles', dataIndex: 'roles', key: 'roles' },
  { title: 'Status', dataIndex: 'active', key: 'active', width: 110 },
  {
    title: 'Last login',
    dataIndex: 'last_login_at',
    key: 'last_login_at',
    width: 200,
  },
  { title: 'Actions', key: 'actions', width: 220 },
];

async function load() {
  loading.value = true;
  try {
    const r = await usersApi.list();
    items.value = r.items ?? [];
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  form.username = '';
  form.email = '';
  form.password = '';
  form.roles = ['operator'];
  drawerOpen.value = true;
}

async function submit() {
  if (!form.username || !form.email || !form.password) {
    message.warning('Username, email and password are required');
    return;
  }
  submitting.value = true;
  try {
    await usersApi.create({
      username: form.username.trim(),
      email: form.email.trim(),
      password: form.password,
      roles: form.roles,
    });
    message.success('User created');
    drawerOpen.value = false;
    await load();
  } finally {
    submitting.value = false;
  }
}

async function resetMfa(id: number) {
  await usersApi.resetMfa(id);
  message.success('MFA reset');
}

async function removeRow(id: number) {
  await usersApi.remove(id);
  message.success('User removed');
  await load();
}

function openChangePassword(record: User) {
  passwordTarget.value = record;
  passwordForm.oldPassword = '';
  passwordForm.newPassword = '';
  passwordForm.confirmPassword = '';
  passwordDrawerOpen.value = true;
}

async function submitChangePassword() {
  const target = passwordTarget.value;
  if (!target) {
    return;
  }
  if (passwordIsSelf.value && !passwordForm.oldPassword) {
    message.warning('Current password is required');
    return;
  }
  if (!passwordForm.newPassword || passwordForm.newPassword.length < 12) {
    message.warning('New password must be at least 12 characters');
    return;
  }
  if (passwordForm.newPassword !== passwordForm.confirmPassword) {
    message.warning('Passwords do not match');
    return;
  }
  passwordSubmitting.value = true;
  try {
    await usersApi.changePassword(target.id, {
      old_password: passwordIsSelf.value ? passwordForm.oldPassword : undefined,
      new_password: passwordForm.newPassword,
    });
    message.success('Password updated');
    passwordDrawerOpen.value = false;
    passwordTarget.value = null;
  } finally {
    passwordSubmitting.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page title="Users" description="Operators with access to this management console">
    <Card :body-style="{ padding: '16px' }">
      <Space class="mb-3">
        <Button type="primary" @click="openCreate">New user</Button>
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
          <template v-if="column.key === 'roles'">
            <Space wrap>
              <Tag v-for="r in record.roles" :key="r" color="blue">{{ r }}</Tag>
            </Space>
          </template>
          <template v-else-if="column.key === 'active'">
            <Tag :color="record.active ? 'green' : 'default'">
              {{ record.active ? 'Active' : 'Disabled' }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Space>
              <Button size="small" @click="openChangePassword(record as User)">
                Change password
              </Button>
              <Popconfirm
                title="Reset this user's MFA? They'll need to re-enroll."
                ok-text="Reset MFA"
                ok-type="danger"
                @confirm="resetMfa(record.id)"
              >
                <Button size="small">Reset MFA</Button>
              </Popconfirm>
              <Popconfirm
                title="Remove this user?"
                ok-text="Remove"
                ok-type="danger"
                @confirm="removeRow(record.id)"
              >
                <Button danger size="small">Remove</Button>
              </Popconfirm>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="drawerOpen"
      title="New user"
      width="460"
      :destroy-on-close="true"
    >
      <Form :model="form" layout="vertical">
        <FormItem
          label="Username"
          name="username"
          :rules="[{ required: true, message: 'Username is required' }]"
        >
          <Input v-model:value="form.username" />
        </FormItem>
        <FormItem
          label="Email"
          name="email"
          :rules="[{ required: true, type: 'email', message: 'A valid email is required' }]"
        >
          <Input v-model:value="form.email" type="email" />
        </FormItem>
        <FormItem
          label="Initial password"
          name="password"
          :rules="[{ required: true, min: 8, message: 'At least 8 characters' }]"
        >
          <Input.Password v-model:value="form.password" />
        </FormItem>
        <FormItem label="Roles" name="roles">
          <Select v-model:value="form.roles" mode="multiple" :options="ROLE_OPTIONS" />
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

    <Drawer
      v-model:open="passwordDrawerOpen"
      :title="
        passwordTarget
          ? `Change password – ${passwordTarget.username}`
          : 'Change password'
      "
      width="460"
      :destroy-on-close="true"
    >
      <Form :model="passwordForm" layout="vertical">
        <FormItem
          v-if="passwordIsSelf"
          label="Current password"
          name="oldPassword"
          :rules="[{ required: true, message: 'Current password is required' }]"
        >
          <Input.Password v-model:value="passwordForm.oldPassword" />
        </FormItem>
        <FormItem
          label="New password"
          name="newPassword"
          :rules="[{ required: true, min: 12, message: 'At least 12 characters' }]"
        >
          <Input.Password v-model:value="passwordForm.newPassword" />
        </FormItem>
        <FormItem
          label="Confirm new password"
          name="confirmPassword"
          :rules="[{ required: true, message: 'Please re-enter the password' }]"
        >
          <Input.Password v-model:value="passwordForm.confirmPassword" />
        </FormItem>
      </Form>
      <template #extra>
        <Space>
          <Button @click="passwordDrawerOpen = false">Cancel</Button>
          <Button
            type="primary"
            :loading="passwordSubmitting"
            @click="submitChangePassword"
          >
            Update
          </Button>
        </Space>
      </template>
    </Drawer>
  </Page>
</template>
