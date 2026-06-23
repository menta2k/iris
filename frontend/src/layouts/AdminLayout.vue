<script setup lang="ts">
import { RouterView, useRouter } from 'vue-router'
import SidebarNav from '@/components/navigation/SidebarNav.vue'
import ConfigDriftBanner from '@/components/common/ConfigDriftBanner.vue'
import { Toaster } from '@/components/ui/toast'
import { Button } from '@/components/ui/button'
import TimezonePicker from '@/components/common/TimezonePicker.vue'
import { useAuth } from '@/composables/useAuth'

const { user, role, logout } = useAuth()
const router = useRouter()

async function onLogout() {
  await logout()
  router.replace({ name: 'login' })
}
</script>

<template>
  <div class="flex min-h-screen flex-col bg-background">
    <!-- Topbar -->
    <header
      class="sticky top-0 z-30 flex h-14 items-center justify-between border-b bg-card/80 px-4 backdrop-blur"
    >
      <div class="flex items-center gap-2">
        <div class="flex h-7 w-7 items-center justify-center rounded bg-primary text-sm font-bold text-primary-foreground">
          I
        </div>
        <div class="leading-tight">
          <p class="text-sm font-semibold">Iris</p>
          <p class="text-[10px] uppercase tracking-widest text-muted-foreground">
            KumoMTA Admin
          </p>
        </div>
      </div>

      <div class="flex items-center gap-4">
        <TimezonePicker />
        <div class="text-right leading-tight">
          <p class="text-xs font-medium">{{ user?.displayName || user?.email }}</p>
          <p class="text-[10px] uppercase tracking-wide text-muted-foreground">{{ role }}</p>
        </div>
        <Button variant="outline" size="sm" @click="onLogout">Sign out</Button>
      </div>
    </header>

    <!-- Pending-config reminder -->
    <ConfigDriftBanner class="sticky top-14 z-20" />

    <div class="flex flex-1">
      <!-- Sidebar -->
      <aside class="w-60 shrink-0 border-r bg-card/40">
        <div class="sticky top-14 max-h-[calc(100vh-3.5rem)] overflow-y-auto">
          <SidebarNav />
        </div>
      </aside>

      <!-- Main content -->
      <main class="flex-1 overflow-x-auto px-6 py-6">
        <RouterView />
      </main>
    </div>

    <Toaster />
  </div>
</template>
