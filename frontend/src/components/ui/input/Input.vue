<script setup lang="ts">
// Thin wrapper over v-text-field (P3). Renders a real <input> that keeps
// the given id (for <Label for>) and native input events, so existing
// pages and component tests keep working.
defineProps<{
  modelValue?: string | number
  type?: string
  placeholder?: string
  disabled?: boolean
  class?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()
</script>

<template>
  <v-text-field
    :model-value="modelValue"
    :type="type ?? 'text'"
    :placeholder="placeholder"
    :disabled="disabled"
    variant="outlined"
    density="compact"
    hide-details
    :class="$props.class"
    @update:model-value="emit('update:modelValue', $event)"
  />
</template>
