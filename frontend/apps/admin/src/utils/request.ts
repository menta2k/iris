import { useAppConfig } from '@vben/hooks';
import { preferences } from '@vben/preferences';
import {
  authenticateResponseInterceptor,
  errorMessageResponseInterceptor,
  RequestClient,
} from '@vben/request';
import { useAccessStore } from '@vben/stores';

import { message } from 'ant-design-vue';

import { useAuthStore } from '#/stores';

const { apiURL } = useAppConfig(import.meta.env, import.meta.env.PROD);

function defaultIdGenerator(): string {
  try {
    const rnd = (globalThis as any)?.crypto?.randomUUID?.();
    if (typeof rnd === 'string' && rnd.length > 0) return rnd;
  } catch {
    // ignore — fall through to the math-based fallback
  }
  return Math.random().toString(36).slice(2) + Date.now().toString(36);
}

function createRequestClient(baseURL: string) {
  const client = new RequestClient({
    baseURL,
  });

  async function doReAuthenticate() {
    const authStore = useAuthStore();
    await authStore.reauthenticate();
  }

  async function doRefreshToken() {
    const authStore = useAuthStore();
    return await authStore.refreshToken();
  }

  function formatToken(token: null | string) {
    return token ? `Bearer ${token}` : null;
  }

  client.addRequestInterceptor({
    fulfilled: async (config) => {
      const accessStore = useAccessStore();
      const requestId =
        config.headers['X-Request-ID'] || defaultIdGenerator();
      (config as any)._requestId = requestId;
      config.headers.Authorization = formatToken(accessStore.accessToken);
      config.headers['Accept-Language'] = preferences.app.locale;
      config.headers['X-Request-ID'] = requestId;
      config.headers['X-Requested-With'] = 'XMLHttpRequest';
      return config;
    },
  });

  // The kumomta backend returns the resource directly on 2xx (no envelope).
  // On non-2xx it returns { code, reason, message } — caught by the
  // errorMessageResponseInterceptor below.
  client.addResponseInterceptor({
    fulfilled: (response) => {
      const { data, status } = response;
      if (status >= 200 && status < 400) {
        return data;
      }
      throw Object.assign({}, response, { response });
    },
  });

  client.addResponseInterceptor(
    authenticateResponseInterceptor({
      client,
      doReAuthenticate,
      doRefreshToken,
      enableRefreshToken: preferences.app.enableRefreshToken,
      formatToken,
    }),
  );

  client.addResponseInterceptor(
    errorMessageResponseInterceptor(async (msg: string, error) => {
      const responseData = error?.response?.data ?? {};
      const errorMessage =
        responseData?.message ??
        responseData?.reason ??
        responseData?.error ??
        '';
      await message.error(errorMessage || msg);
    }),
  );

  return client;
}

export const requestClient = createRequestClient(apiURL);

export const baseRequestClient = new RequestClient({ baseURL: apiURL });
