<script setup lang="ts">
// Plain element inside the v-table wrapper (P3): styling comes from v-table.
defineProps<{ class?: string }>()
</script>

<template>
  <th :class="['text-left', $props.class]">
    <slot />
  </th>
</template>
