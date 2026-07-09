<script setup lang="ts">
import { computed } from 'vue'
import StatTile from './StatTile.vue'

// deferred is an int64 count which proto-JSON serializes as a string. These are
// messages still in the queue after a transient failure — retrying, not bounced.
const props = defineProps<{ deferred?: string | number }>()
const count = computed(() => Number(props.deferred ?? 0))
const formatted = computed(() => count.value.toLocaleString())
</script>

<template>
  <StatTile
    data-testid="deferred-queue-widget"
    label="Deferred in Queue"
    :value="formatted"
    caption="Transient failures, retrying"
    icon="mdi-clock-alert-outline"
    :color="count > 0 ? 'warning' : 'success'"
    :value-class="count > 0 ? 'text-warning' : ''"
  />
</template>
