import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import vuetify from 'vite-plugin-vuetify'

export default defineConfig({
  // vite-plugin-vuetify resolves v-btn/v-dialog/… in component templates,
  // exactly like the app build; without it they render as unknown elements.
  plugins: [vue(), vuetify({ autoImport: true })],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    include: ['tests/component/**/*.spec.ts', 'tests/unit/**/*.spec.ts'],
    setupFiles: ['tests/component/setup.ts'],
    // Vuetify's component modules import raw .css; inline them so vite
    // processes (and stubs) those imports instead of Node failing on them.
    server: { deps: { inline: ['vuetify'] } },
  },
})
