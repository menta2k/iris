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
    class="d-flex align-center justify-center ga-3 py-12 text-body-2 text-medium-emphasis"
  >
    <v-progress-circular indeterminate size="20" width="2" />
    Loading…
  </div>
  <v-alert v-else-if="error" type="error" variant="tonal" density="comfortable" class="text-center">
    {{ error }}
  </v-alert>
  <v-alert
    v-else-if="notImplemented"
    type="warning"
    variant="tonal"
    density="comfortable"
    class="text-center"
  >
    This feature is not available yet. The backend endpoint is still in progress.
  </v-alert>
  <div
    v-else-if="empty"
    class="border border-dashed rounded px-4 py-10 text-center text-body-2 text-medium-emphasis"
  >
    {{ emptyMessage ?? 'No records found.' }}
  </div>
  <slot v-else />
</template>
