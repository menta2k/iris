<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { navSections } from './nav-items'
import { useAuth } from '@/composables/useAuth'

const { hasPermission } = useAuth()

const visibleSections = computed(() =>
  navSections
    .map((section) => ({
      ...section,
      items: section.items.filter((item) => hasPermission(item.permission)),
    }))
    .filter((section) => section.items.length > 0),
)
</script>

<template>
  <nav class="flex flex-col gap-6 px-3 py-4">
    <div v-for="section in visibleSections" :key="section.label">
      <p class="px-3 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        {{ section.label }}
      </p>
      <ul class="space-y-0.5">
        <li v-for="item in section.items" :key="item.to">
          <RouterLink
            :to="item.to"
            class="block rounded-md px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            active-class="bg-accent text-foreground font-medium"
            exact-active-class="bg-accent text-foreground font-medium"
          >
            {{ item.label }}
          </RouterLink>
        </li>
      </ul>
    </div>
  </nav>
</template>
