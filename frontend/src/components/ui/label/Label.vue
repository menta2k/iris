<script setup lang="ts">
defineProps<{ for?: string; class?: string }>()
</script>

<template>
  <label :for="$props.for" class="text-body-2 font-weight-medium" :class="$props.class">
    <slot />
  </label>
</template>
