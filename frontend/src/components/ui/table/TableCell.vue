<script setup lang="ts">
import { cn } from '@/lib/utils'
defineProps<{ class?: string }>()
</script>

<template>
  <td :class="cn('px-3 py-2 align-middle [&:has([role=checkbox])]:pr-0', $props.class)">
    <slot />
  </td>
</template>
