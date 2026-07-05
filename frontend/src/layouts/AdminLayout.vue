<script setup lang="ts">
import { ref, watch } from 'vue'
import { RouterView, useRouter } from 'vue-router'
import { useDisplay } from 'vuetify'
import SidebarNav from '@/components/navigation/SidebarNav.vue'
import ConfigDriftBanner from '@/components/common/ConfigDriftBanner.vue'
import SettingsDrawer from '@/components/common/SettingsDrawer.vue'
import { Toaster } from '@/components/ui/toast'
import TimezonePicker from '@/components/common/TimezonePicker.vue'
import { useAuth } from '@/composables/useAuth'
import { useThemeSync } from '@/composables/useThemeSync'
import { useConfigStore } from '@/stores/config'

const { user, role, logout } = useAuth()
const router = useRouter()
const { mdAndUp } = useDisplay()
const config = useConfigStore()
useThemeSync()

const settingsOpen = ref(false)

// Permanent (always visible) on desktop; overlay toggled from the app bar
// on small screens.
const drawer = ref(mdAndUp.value)
watch(mdAndUp, (isDesktop) => {
  drawer.value = isDesktop
})

async function onLogout() {
  await logout()
  router.replace({ name: 'login' })
}
</script>

<template>
  <v-app :class="{ 'skin-bordered': config.skin === 'bordered' }">
    <v-navigation-drawer
      v-model="drawer"
      :permanent="mdAndUp"
      :temporary="!mdAndUp"
      width="260"
    >
      <template #prepend>
        <div class="d-flex align-center ga-2 px-4 py-4">
          <div
            class="d-flex align-center justify-center rounded bg-primary text-body-2 font-weight-bold"
            style="width: 28px; height: 28px"
          >
            I
          </div>
          <div>
            <p class="text-body-2 font-weight-bold">Iris</p>
            <p class="text-caption text-uppercase text-medium-emphasis">
              KumoMTA Admin
            </p>
          </div>
        </div>
      </template>
      <SidebarNav />
    </v-navigation-drawer>

    <v-app-bar flat border density="comfortable">
      <v-app-bar-nav-icon v-if="!mdAndUp" @click="drawer = !drawer" />
      <v-spacer />
      <div class="d-flex align-center ga-4 pr-4">
        <TimezonePicker />
        <div class="text-right">
          <p class="text-caption font-weight-medium">{{ user?.displayName || user?.email }}</p>
          <p class="text-caption text-uppercase text-medium-emphasis">{{ role }}</p>
        </div>
        <v-btn
          icon="mdi-cog-outline"
          variant="text"
          size="small"
          aria-label="Open theme settings"
          @click="settingsOpen = !settingsOpen"
        />
        <v-btn variant="outlined" size="small" color="primary" @click="onLogout">
          Sign out
        </v-btn>
      </div>
    </v-app-bar>

    <SettingsDrawer v-model="settingsOpen" />

    <v-main>
      <ConfigDriftBanner />
      <div class="px-6 py-6">
        <RouterView />
      </div>
    </v-main>

    <Toaster />
  </v-app>
</template>
