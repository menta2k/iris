<script setup lang="ts">
defineProps<{ class?: string }>()
</script>

<template>
  <div class="pa-4 pt-0" :class="$props.class">
    <slot />
  </div>
</template>
