import { defineConfig } from '@vben/vite-config';

export default defineConfig(async () => {
  return {
    application: {},
    vite: {
      server: {
        proxy: {
          // The kumomta admin-service registers HTTP routes at /v1/*.
          // The SPA's request client uses /api as its baseURL, so we rewrite
          // /api/v1/foo → /v1/foo before forwarding to the backend.
          '/api': {
            changeOrigin: true,
            rewrite: (path) => path.replace(/^\/api/, ''),
            target: 'http://127.0.0.1:8000',
            ws: true,
          },
        },
      },
    },
  };
});
