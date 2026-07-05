<script setup lang="ts">
defineProps<{ class?: string }>()
</script>

<template>
  <p class="text-body-2 text-medium-emphasis" :class="$props.class">
    <slot />
  </p>
</template>
