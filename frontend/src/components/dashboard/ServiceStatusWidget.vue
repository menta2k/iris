<script setup lang="ts">
import { computed } from 'vue'
import StatTile from './StatTile.vue'

const props = defineProps<{ state?: string }>()

// Map the daemon state onto a semantic color; anything unexpected reads as a
// warning rather than silently looking healthy.
const color = computed(() => {
  const s = (props.state ?? '').toLowerCase()
  if (['running', 'ok', 'healthy', 'active'].includes(s)) return 'success'
  if (['stopped', 'failed', 'error', 'dead'].includes(s)) return 'error'
  return 'warning'
})
</script>

<template>
  <StatTile
    data-testid="service-status-widget"
    label="KumoMTA Service"
    :value="state ?? 'Unknown'"
    caption="MTA daemon state"
    icon="mdi-server-outline"
    :color="color"
  />
</template>
