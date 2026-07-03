import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import { vuetify } from '@/plugins/vuetify'
import { restoreSession } from '@/composables/useAuth'
import './style.css'

// Restore any saved session before mounting so the first navigation already
// knows whether the user is authenticated (avoids a login flash on reload).
restoreSession().finally(() => {
  createApp(App).use(createPinia()).use(router).use(vuetify).mount('#app')
})
