<script setup lang="ts">
import { computed } from 'vue'
import Badge from './Badge.vue'

const props = defineProps<{ status?: string }>()

const variant = computed(() => {
  const s = (props.status ?? '').toUpperCase()
  if (['ACTIVE', 'RUNNING', 'SUCCESS', 'DELIVERED', 'SENT', 'HEALTHY', 'COMPLETED', 'ENABLED'].includes(s))
    return 'success' as const
  // Intake / informational states get the primary tint (distinct from the
  // green "sent/delivered" success states).
  if (['RECEIVED', 'RECEPTION', 'QUEUED', 'ACCEPTED'].includes(s))
    return 'default' as const
  if (['DRAINING', 'PAUSED', 'PENDING', 'WARNING', 'DEFERRED'].includes(s))
    return 'warning' as const
  if (['DISABLED', 'FAILED', 'BOUNCED', 'STOPPED', 'ERROR', 'SUPPRESSED', 'REJECTED'].includes(s))
    return 'destructive' as const
  return 'secondary' as const
})

const label = computed(() => {
  if (!props.status) return 'Unknown'
  return props.status
    .replace(/^STATUS_/, '')
    .replace(/_/g, ' ')
    .toLowerCase()
    .replace(/\b\w/g, (c) => c.toUpperCase())
})
</script>

<template>
  <Badge :variant="variant">{{ label }}</Badge>
</template>
