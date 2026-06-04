<script lang="ts" setup>
import type { VbenFormSchema } from '@vben/common-ui';

import { computed, ref } from 'vue';

import { AuthenticationLogin, z } from '@vben/common-ui';
import { $t } from '@vben/locales';

import { Button, Input, message, Space } from 'ant-design-vue';

import { useAuthStore } from '#/stores';

defineOptions({ name: 'Login' });

const authStore = useAuthStore();

// MFA second-step state (shown when authStore.mfaToken is set).
const useBackup = ref(false);
const code = ref('');

const hasPasskey = computed(() => authStore.mfaMethods.includes('webauthn'));
const hasBackup = computed(() => authStore.mfaMethods.includes('backup_code'));

async function submitMfa() {
  const value = code.value.trim();
  if (!value) {
    message.warning('Enter your code');
    return;
  }
  try {
    await authStore.verifyMfa(useBackup.value ? { backup_code: value } : { code: value });
  } catch {
    // error toast surfaced by the request interceptor; let the user retry
  }
}

async function submitPasskey() {
  try {
    await authStore.verifyPasskey();
  } catch {
    message.error('Passkey verification failed or was cancelled');
  }
}

const formSchema = computed((): VbenFormSchema[] => {
  return [
    {
      component: 'VbenInput',
      componentProps: {
        placeholder: $t('authentication.usernameTip'),
      },
      fieldName: 'username',
      label: $t('authentication.username'),
      rules: z.string().min(1, { message: $t('authentication.usernameTip') }),
    },
    {
      component: 'VbenInputPassword',
      componentProps: {
        placeholder: $t('authentication.password'),
      },
      fieldName: 'password',
      label: $t('authentication.password'),
      rules: z.string().min(1, { message: $t('authentication.passwordTip') }),
    },
  ];
});
</script>

<template>
  <!-- Second factor required: replace the password form with the challenge. -->
  <div v-if="authStore.mfaToken" class="mfa-step">
    <h2 class="mfa-title">Two-factor authentication</h2>
    <p class="mfa-hint">
      {{
        useBackup
          ? 'Enter one of your backup codes.'
          : 'Enter the 6-digit code from your authenticator app.'
      }}
    </p>

    <Space direction="vertical" style="width: 100%" :size="12">
      <Input
        v-model:value="code"
        :placeholder="useBackup ? 'xxxx-xxxx' : '123456'"
        size="large"
        autofocus
        @press-enter="submitMfa"
      />
      <Button
        type="primary"
        size="large"
        block
        :loading="authStore.loginLoading"
        @click="submitMfa"
      >
        Verify
      </Button>

      <Button v-if="hasPasskey" size="large" block @click="submitPasskey">
        Use a passkey
      </Button>

      <div class="mfa-links">
        <a v-if="hasBackup" @click="useBackup = !useBackup">
          {{ useBackup ? 'Use authenticator code' : 'Use a backup code' }}
        </a>
        <a @click="authStore.cancelMfa()">Back to login</a>
      </div>
    </Space>
  </div>

  <AuthenticationLogin
    v-else
    :form-schema="formSchema"
    :loading="authStore.loginLoading"
    :show-code-login="false"
    :show-forget-password="false"
    :show-qrcode-login="false"
    :show-register="true"
    :show-third-party-login="false"
    @submit="authStore.authLogin"
  />
</template>

<style scoped>
.mfa-step {
  width: 100%;
  max-width: 360px;
  margin: 0 auto;
}
.mfa-title {
  margin-bottom: 8px;
  font-size: 22px;
  font-weight: 600;
}
.mfa-hint {
  margin-bottom: 20px;
  color: var(--ant-color-text-tertiary);
}
.mfa-links {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
}
.mfa-links a {
  cursor: pointer;
}
</style>
