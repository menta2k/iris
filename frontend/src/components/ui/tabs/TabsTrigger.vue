<script setup lang="ts">
import { computed, inject, type Ref } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{ value: string }>()
const tabs = inject<{ active: Ref<string>; setActive: (v: string) => void }>('tabs')

const isActive = computed(() => tabs?.active.value === props.value)
</script>

<template>
  <button
    type="button"
    :class="
      cn(
        'inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium transition-all focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
        isActive
          ? 'bg-background text-foreground shadow'
          : 'text-muted-foreground hover:text-foreground',
      )
    "
    @click="tabs?.setActive(value)"
  >
    <slot />
  </button>
</template>
