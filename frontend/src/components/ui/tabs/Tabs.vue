<script setup lang="ts">
import { provide, ref, watch, type Ref } from 'vue'

const props = defineProps<{ modelValue?: string; defaultValue?: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()

const active = ref(props.modelValue ?? props.defaultValue ?? '')

watch(
  () => props.modelValue,
  (v) => {
    if (v !== undefined) active.value = v
  },
)

function setActive(value: string) {
  active.value = value
  emit('update:modelValue', value)
}

provide<{ active: Ref<string>; setActive: (v: string) => void }>('tabs', {
  active,
  setActive,
})
</script>

<template>
  <div>
    <slot />
  </div>
</template>
