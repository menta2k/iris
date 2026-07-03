<script setup lang="ts">
// Thin wrapper over v-table (P3): the default slot renders inside
// Vuetify's <table>, so the thead/tbody/tr sub-wrappers keep working.
withDefaults(
  defineProps<{
    class?: string
    density?: 'default' | 'comfortable' | 'compact'
    fixedHeader?: boolean
    height?: string | number
  }>(),
  { density: 'compact' },
)
</script>

<template>
  <v-table
    :density="density"
    :fixed-header="fixedHeader"
    :height="height"
    hover
    :class="$props.class"
  >
    <slot />
  </v-table>
</template>

<style scoped lang="scss">
// Give row action buttons a shared minimum width so paired buttons line up
// into tidy columns across rows (e.g. Suspend/Resume occupy the same slot,
// so their left edges stay flush). Icon buttons keep their round shape.
:deep(tbody td .v-btn:not(.v-btn--icon)) {
  min-width: 90px;
}
</style>
