import type { Recordable, UserInfo } from '@vben/types';

import { ref } from 'vue';
import { useRouter } from 'vue-router';

import { DEFAULT_HOME_PATH, LOGIN_PATH } from '@vben/constants';
import { resetAllStores, useAccessStore, useUserStore } from '@vben/stores';

import {
  startAuthentication,
} from '@simplewebauthn/browser';
import { notification } from 'ant-design-vue';
import { defineStore } from 'pinia';

import { mfaApi } from '#/api/kumo';
import { requestClient } from '#/utils/request';

interface LoginResponse {
  access_token: string;
  refresh_token: string;
  user_id: number;
  username: string;
  roles?: string[];
  // Second-factor challenge (instead of tokens).
  mfa_required?: boolean;
  mfa_token?: string;
  mfa_methods?: string[];
}

interface AuthLoginResult {
  userInfo: null | UserInfo;
}

function toUserInfo(resp: LoginResponse): UserInfo {
  const roles = resp.roles ?? [];
  return {
    avatar: '',
    description: '',
    homePath: DEFAULT_HOME_PATH,
    id: resp.user_id,
    nickname: resp.username,
    realname: resp.username,
    roles,
    tenantId: 0,
    token: resp.access_token,
    username: resp.username,
  };
}

export const useAuthStore = defineStore('auth', () => {
  const accessStore = useAccessStore();
  const userStore = useUserStore();
  const router = useRouter();

  const loginLoading = ref(false);

  // Pending second-factor challenge. When mfaToken is set the login view
  // swaps to the code/passkey step instead of navigating.
  const mfaToken = ref<string>('');
  const mfaMethods = ref<string[]>([]);

  function applyLoginResponse(resp: LoginResponse): UserInfo {
    const userInfo = toUserInfo(resp);
    accessStore.setAccessToken(resp.access_token);
    accessStore.setRefreshToken(resp.refresh_token);
    accessStore.setAccessCodes(userInfo.roles ?? []);
    userStore.setUserInfo(userInfo);
    return userInfo;
  }

  async function authLogin(
    params: Recordable<any>,
    onSuccess?: () => Promise<void> | void,
  ): Promise<AuthLoginResult | null> {
    let userInfo: null | UserInfo = null;
    try {
      loginLoading.value = true;

      const resp = await requestClient.post<LoginResponse>('/v1/auth/login', {
        username: params.username,
        password: params.password,
      });

      // Password OK but a second factor is required: hold the challenge and
      // let the login view render the code/passkey step. No tokens yet.
      if (resp.mfa_required && resp.mfa_token) {
        mfaToken.value = resp.mfa_token;
        mfaMethods.value = resp.mfa_methods ?? [];
        return { userInfo: null };
      }

      userInfo = finishLogin(resp);
    } finally {
      loginLoading.value = false;
    }

    if (userInfo && onSuccess) {
      await onSuccess();
    } else if (userInfo) {
      await router.push(userInfo.homePath || DEFAULT_HOME_PATH);
    }

    return { userInfo };
  }

  // finishLogin applies tokens, clears any MFA challenge, and greets the user.
  function finishLogin(resp: LoginResponse): UserInfo {
    const userInfo = applyLoginResponse(resp);
    accessStore.setLoginExpired(false);
    mfaToken.value = '';
    mfaMethods.value = [];
    if (userInfo.realname) {
      notification.success({
        description: `Welcome back, ${userInfo.realname}`,
        duration: 3,
        message: 'Login successful',
      });
    }
    return userInfo;
  }

  // verifyMfa completes a TOTP / backup-code challenge.
  async function verifyMfa(body: { backup_code?: string; code?: string }) {
    loginLoading.value = true;
    try {
      const resp = await mfaApi.verify(mfaToken.value, body) as LoginResponse;
      const userInfo = finishLogin(resp);
      await router.push(userInfo.homePath || DEFAULT_HOME_PATH);
    } finally {
      loginLoading.value = false;
    }
  }

  // verifyPasskey runs the WebAuthn login ceremony for the held challenge.
  async function verifyPasskey() {
    loginLoading.value = true;
    try {
      const start = await mfaApi.passkeyLoginStart(mfaToken.value);
      const assertion = await startAuthentication({
        optionsJSON: (start.options as any).publicKey ?? start.options,
      });
      const resp = (await mfaApi.passkeyLoginFinish(
        start.operation_id,
        assertion,
      )) as LoginResponse;
      const userInfo = finishLogin(resp);
      await router.push(userInfo.homePath || DEFAULT_HOME_PATH);
    } finally {
      loginLoading.value = false;
    }
  }

  function cancelMfa() {
    mfaToken.value = '';
    mfaMethods.value = [];
  }

  async function refreshToken(): Promise<string> {
    const refresh = accessStore.refreshToken;
    if (!refresh) {
      return '';
    }
    const resp = await requestClient.post<LoginResponse>('/v1/auth/refresh', {
      refresh_token: refresh,
    });
    applyLoginResponse(resp);
    return resp.access_token ?? '';
  }

  async function reauthenticate(): Promise<void> {
    accessStore.setAccessToken(null);
    accessStore.setRefreshToken(null);
    accessStore.setLoginExpired(true);
    resetAllStores();
    await router.replace({
      path: LOGIN_PATH,
      query: {
        redirect: encodeURIComponent(router.currentRoute.value.fullPath),
      },
    });
  }

  async function logout(redirect: boolean = true): Promise<void> {
    try {
      if (accessStore.accessToken) {
        await requestClient.post('/v1/auth/logout', {});
      }
    } catch {
      // backend logout is best-effort; clear local state regardless
    }
    accessStore.setAccessToken(null);
    accessStore.setRefreshToken(null);
    accessStore.setAccessCodes([]);
    resetAllStores();

    if (redirect) {
      await router.replace({
        path: LOGIN_PATH,
        query: {
          redirect: encodeURIComponent(router.currentRoute.value.fullPath),
        },
      });
    }
  }

  async function getUserPermissionCodes(): Promise<string[]> {
    return userStore.userInfo?.roles ?? accessStore.accessCodes ?? [];
  }

  return {
    authLogin,
    cancelMfa,
    getUserPermissionCodes,
    loginLoading,
    logout,
    mfaMethods,
    mfaToken,
    reauthenticate,
    refreshToken,
    verifyMfa,
    verifyPasskey,
  };
});
