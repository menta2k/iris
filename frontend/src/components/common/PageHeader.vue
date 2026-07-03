<script setup lang="ts">
defineProps<{ title: string; description?: string }>()
</script>

<template>
  <div class="d-flex flex-wrap align-start justify-space-between ga-3 mb-5">
    <div>
      <h1 class="text-h5 font-weight-bold">{{ title }}</h1>
      <p v-if="description" class="mt-1 text-body-2 text-medium-emphasis">{{ description }}</p>
    </div>
    <div class="d-flex align-center ga-2">
      <slot name="actions" />
    </div>
  </div>
</template>
