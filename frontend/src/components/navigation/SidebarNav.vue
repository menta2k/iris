<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { navItems, type NavItem } from './nav-items'
import { useAuth } from '@/composables/useAuth'

const { hasPermission } = useAuth()
const route = useRoute()

function filterByPermission(items: NavItem[]): NavItem[] {
  return items
    .map((item) =>
      item.children ? { ...item, children: filterByPermission(item.children) } : item,
    )
    .filter((item) =>
      item.children ? item.children.length > 0 : hasPermission(item.permission),
    )
}

const visibleItems = computed(() => filterByPermission(navItems))

// True when a group holds the current route — used to tint the header even while
// collapsed, so the user always sees where they are in the menu.
function groupContainsActive(item: NavItem): boolean {
  return !!item.children?.some((child) => child.to === route.path)
}

// Start with the group containing the current page expanded.
const initiallyOpened = navItems
  .filter((item) => item.children?.some((child) => child.to === route.path))
  .map((item) => item.label)
const opened = ref<string[]>(initiallyOpened)
</script>

<template>
  <v-list v-model:opened="opened" nav color="primary" class="sidebar-nav">
    <template v-for="item in visibleItems" :key="item.label">
      <v-list-group v-if="item.children" :value="item.label">
        <template #activator="{ props: activatorProps }">
          <v-list-item
            v-bind="activatorProps"
            :prepend-icon="item.icon"
            :title="item.label"
            class="sidebar-group-header"
            :class="{ 'sidebar-group-header--contains-active': groupContainsActive(item) }"
          />
        </template>
        <v-list-item
          v-for="child in item.children"
          :key="child.to"
          :to="child.to"
          :title="child.label"
          class="sidebar-child"
        />
      </v-list-group>
      <v-list-item
        v-else
        :to="item.to"
        :exact="item.to === '/'"
        :prepend-icon="item.icon"
        :title="item.label"
        class="sidebar-leaf"
      />
    </template>
  </v-list>
</template>

<style scoped lang="scss">
// Vuexy-style vertical nav: rounded pill rows, group headers that tint when they
// contain the active route, and child items hanging off a thin guide rail.

.sidebar-nav {
  $pill-radius: 10px;

  // Every row becomes a rounded pill with comfortable height.
  :deep(.v-list-item) {
    min-height: 42px;
    border-radius: $pill-radius;
    margin-block: 2px;
    font-weight: 500;
    letter-spacing: 0.1px;
  }

  // Top-level leaf (Dashboard) reads slightly heavier.
  :deep(.sidebar-leaf) {
    font-weight: 600;
  }

  // Group header = section opener. Tint it (icon + title) when one of its
  // children is the active route, so the location is visible while collapsed.
  :deep(.sidebar-group-header) {
    font-weight: 600;
  }
  :deep(.sidebar-group-header--contains-active) {
    color: rgb(var(--v-theme-primary));
  }
  :deep(.sidebar-group-header--contains-active .v-icon) {
    color: rgb(var(--v-theme-primary));
  }

  // Child items: lighter weight + smaller type to read as nested sub-items.
  :deep(.sidebar-child) {
    min-height: 38px;
    font-size: 0.875rem;
    font-weight: 400;
    // Vuetify indents nested items via --indent-padding (defaults to ~48px,
    // pushing children far to the right). Shrink it so a child hangs just
    // past the guide rail instead of floating out in empty space.
    --indent-padding: 16px;
  }

  // The indented guide rail connecting a group's children, like Vuexy's nav.
  // Rail sits roughly under the parent icon; children hang close to it.
  :deep(.v-list-group__items) {
    position: relative;
    margin-inline-start: 22px;
    margin-inline-end: 8px;
    padding-inline-start: 2px;
    border-inline-start: 2px solid rgba(var(--v-theme-on-surface), 0.1);
  }
}
</style>
