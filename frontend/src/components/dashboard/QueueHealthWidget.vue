<script setup lang="ts">
import { computed } from 'vue'
import StatTile from './StatTile.vue'

// queued is an int64 count which proto-JSON serializes as a string.
const props = defineProps<{ queued?: string | number }>()
const formatted = computed(() => Number(props.queued ?? 0).toLocaleString())
</script>

<template>
  <StatTile
    data-testid="queue-health-widget"
    label="Queued Messages"
    :value="formatted"
    caption="Awaiting delivery"
    icon="mdi-email-multiple-outline"
    color="primary"
  />
</template>
