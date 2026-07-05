<script setup lang="ts">
defineProps<{ class?: string }>()
</script>

<template>
  <h3 class="text-subtitle-1 font-weight-bold" :class="$props.class">
    <slot />
  </h3>
</template>
