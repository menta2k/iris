import { createVuetify } from 'vuetify'
import { aliases, mdi } from 'vuetify/iconsets/mdi'
// vite-plugin-vuetify rewrites this import to a SASS build using
// src/styles/settings.scss ($layers: true).
import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'
import { themes } from './theme'

// Vuetify styles are compiled from SASS with $layers: true (see
// src/styles/settings.scss + vite-plugin-vuetify) so they live in the
// `vuetify` cascade layer and coexist with Tailwind — layer order is
// declared in src/style.css.
export const vuetify = createVuetify({
  icons: {
    defaultSet: 'mdi',
    aliases,
    sets: { mdi },
  },
  theme: {
    // index.html hard-codes class="dark" (Tailwind); keep both systems in
    // sync until the P2 settings drawer owns the mode switch.
    defaultTheme: 'dark',
    themes,
  },
  defaults: {
    VBtn: { style: 'text-transform: none;' },
  },
})
