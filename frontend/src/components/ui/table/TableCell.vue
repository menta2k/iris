<script setup lang="ts">
// Plain element inside the v-table wrapper (P3): styling comes from v-table.
defineProps<{ class?: string }>()
</script>

<template>
  <td :class="$props.class">
    <slot />
  </td>
</template>
