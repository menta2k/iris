import type { Recordable, UserInfo } from '@vben/types';

import { ref } from 'vue';
import { useRouter } from 'vue-router';

import { DEFAULT_HOME_PATH, LOGIN_PATH } from '@vben/constants';
import { resetAllStores, useAccessStore, useUserStore } from '@vben/stores';

import { notification } from 'ant-design-vue';
import { defineStore } from 'pinia';

import { requestClient } from '#/utils/request';

interface LoginResponse {
  access_token: string;
  refresh_token: string;
  user_id: number;
  username: string;
  roles?: string[];
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

      userInfo = applyLoginResponse(resp);
      accessStore.setLoginExpired(false);

      if (onSuccess) {
        await onSuccess();
      } else {
        await router.push(userInfo.homePath || DEFAULT_HOME_PATH);
      }

      if (userInfo.realname) {
        notification.success({
          description: `Welcome back, ${userInfo.realname}`,
          duration: 3,
          message: 'Login successful',
        });
      }
    } finally {
      loginLoading.value = false;
    }

    return { userInfo };
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
    getUserPermissionCodes,
    loginLoading,
    logout,
    reauthenticate,
    refreshToken,
  };
});
