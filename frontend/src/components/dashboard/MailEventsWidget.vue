<script setup lang="ts">
import { computed } from 'vue'
import StatTile from './StatTile.vue'

// count is an int64 which proto-JSON serializes as a string.
const props = defineProps<{ count?: string | number }>()
const formatted = computed(() => Number(props.count ?? 0).toLocaleString())
</script>

<template>
  <StatTile
    data-testid="mail-events-widget"
    label="Mail Events"
    :value="formatted"
    caption="Last hour"
    icon="mdi-lightning-bolt-outline"
    color="info"
  />
</template>
