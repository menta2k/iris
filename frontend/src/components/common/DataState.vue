<script setup lang="ts">
defineProps<{
  loading?: boolean
  error?: string | null
  notImplemented?: boolean
  empty?: boolean
  emptyMessage?: string
}>()
</script>

<template>
  <div
    v-if="loading"
    class="flex items-center justify-center py-12 text-sm text-muted-foreground"
  >
    Loading…
  </div>
  <div
    v-else-if="error"
    class="rounded-md border border-destructive/40 bg-destructive/5 px-4 py-8 text-center text-sm text-destructive"
  >
    {{ error }}
  </div>
  <div
    v-else-if="notImplemented"
    class="rounded-md border border-warning/40 bg-warning/5 px-4 py-8 text-center text-sm text-warning"
  >
    This feature is not available yet. The backend endpoint is still in progress.
  </div>
  <div
    v-else-if="empty"
    class="rounded-md border border-dashed px-4 py-10 text-center text-sm text-muted-foreground"
  >
    {{ emptyMessage ?? 'No records found.' }}
  </div>
  <slot v-else />
</template>
