<script lang="ts" setup>
import type { MFAStatus, TOTPEnrollStart } from '#/api/kumo';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { startRegistration } from '@simplewebauthn/browser';
import {
  Alert,
  Button,
  Card,
  Input,
  message,
  Modal,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
} from 'ant-design-vue';

import { mfaApi } from '#/api/kumo';

defineOptions({ name: 'MfaSettings' });

const loading = ref(false);
const status = ref<MFAStatus | null>(null);

// TOTP enrollment state.
const enrolling = ref<null | TOTPEnrollStart>(null);
const totpCode = ref('');
const submitting = ref(false);

// Backup codes shown once after generation.
const backupCodes = ref<string[]>([]);
const backupModalOpen = ref(false);

async function load() {
  loading.value = true;
  try {
    status.value = await mfaApi.status();
  } finally {
    loading.value = false;
  }
}

async function startTotp() {
  try {
    enrolling.value = await mfaApi.totpEnroll();
    totpCode.value = '';
  } catch {
    // interceptor shows the error (e.g. MFA not configured on the server)
  }
}

async function confirmTotp() {
  if (!enrolling.value) return;
  if (!totpCode.value.trim()) {
    message.warning('Enter the 6-digit code');
    return;
  }
  submitting.value = true;
  try {
    const resp = await mfaApi.totpConfirm(
      enrolling.value.operation_id,
      totpCode.value.trim(),
    );
    enrolling.value = null;
    showBackupCodes(resp.backup_codes);
    message.success('Authenticator enabled');
    await load();
  } finally {
    submitting.value = false;
  }
}

async function addPasskey() {
  try {
    const start = await mfaApi.passkeyEnrollStart();
    const attestation = await startRegistration({
      optionsJSON: (start.options as any).publicKey ?? start.options,
    });
    const label =
      window.prompt('Name this passkey (e.g. "YubiKey", "MacBook")') ?? 'passkey';
    await mfaApi.passkeyEnrollFinish(start.operation_id, attestation, label);
    message.success('Passkey added');
    await load();
  } catch {
    message.error('Passkey registration failed or was cancelled');
  }
}

async function removePasskey(id: number) {
  await mfaApi.removePasskey(id);
  message.success('Passkey removed');
  await load();
}

async function regenerateBackup() {
  const resp = await mfaApi.regenerateBackupCodes();
  showBackupCodes(resp.backup_codes);
  await load();
}

async function disableMfa() {
  await mfaApi.disable();
  message.success('MFA disabled');
  await load();
}

function showBackupCodes(codes: string[]) {
  backupCodes.value = codes;
  backupModalOpen.value = true;
}

const passkeyColumns = [
  { title: 'Name', dataIndex: 'label', key: 'label' },
  { title: 'Added', dataIndex: 'created_at', key: 'created_at', width: 220 },
  { title: 'Actions', key: 'actions', width: 120 },
];

onMounted(load);
</script>

<template>
  <Page
    title="My MFA"
    description="Add a second factor to your account. You'll be asked for it after your password at sign-in."
  >
    <!-- Authenticator app (TOTP) -->
    <Card title="Authenticator app (TOTP)" :body-style="{ padding: '20px' }" class="mb-4">
      <template v-if="status?.totp_enabled && !enrolling">
        <Space>
          <Tag color="green">Enabled</Tag>
          <Button danger size="small" @click="startTotp">Re-enroll</Button>
        </Space>
      </template>

      <template v-else-if="enrolling">
        <Space direction="vertical" :size="12" style="width: 100%; max-width: 360px">
          <Alert
            type="info"
            show-icon
            message="Scan the QR code with your authenticator app, then enter the 6-digit code to confirm."
          />
          <img :src="enrolling.qr_code_data_uri" alt="TOTP QR code" width="200" />
          <Typography.Text type="secondary">
            Or enter this secret manually:
            <Typography.Text code copyable>{{ enrolling.secret }}</Typography.Text>
          </Typography.Text>
          <Input v-model:value="totpCode" placeholder="123456" />
          <Space>
            <Button type="primary" :loading="submitting" @click="confirmTotp">
              Confirm
            </Button>
            <Button @click="enrolling = null">Cancel</Button>
          </Space>
        </Space>
      </template>

      <template v-else>
        <Space>
          <Tag>Not enabled</Tag>
          <Button type="primary" size="small" @click="startTotp">Set up</Button>
        </Space>
      </template>
    </Card>

    <!-- Passkeys (WebAuthn) -->
    <Card title="Passkeys / security keys" :body-style="{ padding: '16px 20px' }" class="mb-4">
      <Alert
        v-if="status && !status.webauthn_enabled"
        type="warning"
        show-icon
        class="mb-3"
        message="Passkeys are not configured on this server (set IRIS_WEBAUTHN_RP_ID)."
      />
      <Space class="mb-3">
        <Button
          type="primary"
          :disabled="!status?.webauthn_enabled"
          @click="addPasskey"
        >
          Add passkey
        </Button>
      </Space>
      <Table
        :columns="passkeyColumns"
        :data-source="status?.passkeys ?? []"
        :loading="loading"
        :pagination="false"
        row-key="id"
        size="middle"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'actions'">
            <Popconfirm
              title="Remove this passkey?"
              ok-text="Remove"
              ok-type="danger"
              @confirm="removePasskey(record.id)"
            >
              <Button danger size="small">Remove</Button>
            </Popconfirm>
          </template>
        </template>
      </Table>
    </Card>

    <!-- Backup codes + disable -->
    <Card title="Backup codes" :body-style="{ padding: '20px' }" class="mb-4">
      <Space direction="vertical" :size="12">
        <Typography.Text>
          Remaining single-use backup codes:
          <Tag :color="(status?.backup_remaining ?? 0) > 0 ? 'blue' : 'orange'">
            {{ status?.backup_remaining ?? 0 }}
          </Tag>
        </Typography.Text>
        <Space>
          <Button @click="regenerateBackup">Regenerate backup codes</Button>
          <Popconfirm
            title="Disable all MFA on your account?"
            ok-text="Disable"
            ok-type="danger"
            @confirm="disableMfa"
          >
            <Button danger>Disable MFA</Button>
          </Popconfirm>
        </Space>
      </Space>
    </Card>

    <Modal
      v-model:open="backupModalOpen"
      title="Your backup codes"
      :footer="null"
      @cancel="backupModalOpen = false"
    >
      <Alert
        type="warning"
        show-icon
        class="mb-3"
        message="Store these now — they are shown only once. Each code works a single time."
      />
      <pre class="backup-codes">{{ backupCodes.join('\n') }}</pre>
      <Button type="primary" block @click="backupModalOpen = false">Done</Button>
    </Modal>
  </Page>
</template>

<style scoped>
.backup-codes {
  padding: 12px 16px;
  margin-bottom: 16px;
  font-family: var(--font-mono, monospace);
  font-size: 15px;
  letter-spacing: 0.08em;
  background: var(--ant-color-fill-quaternary);
  border-radius: 6px;
}
</style>
