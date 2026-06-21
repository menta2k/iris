<script setup lang="ts">
import { computed, inject, type Ref } from 'vue'

const props = defineProps<{ value: string }>()
const tabs = inject<{ active: Ref<string>; setActive: (v: string) => void }>('tabs')

const isActive = computed(() => tabs?.active.value === props.value)
</script>

<template>
  <div v-if="isActive" class="mt-4">
    <slot />
  </div>
</template>
