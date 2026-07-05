<script setup lang="ts">
defineProps<{ class?: string }>()
</script>

<template>
  <div class="d-flex flex-column ga-1 pa-4" :class="$props.class">
    <slot />
  </div>
</template>
