<script setup lang="ts" generic="T extends string">
// Segmented time-range selector shared by the dashboard panels.
defineProps<{ modelValue: T; options: readonly T[] }>()

const emit = defineEmits<{ 'update:modelValue': [value: T] }>()
</script>

<template>
  <v-btn-toggle
    :model-value="modelValue"
    color="primary"
    density="compact"
    variant="outlined"
    divided
    mandatory
    class="range-toggle"
    @update:model-value="(v: T) => emit('update:modelValue', v)"
  >
    <v-btn
      v-for="option in options"
      :key="option"
      :value="option"
      size="small"
      class="text-caption font-weight-medium"
    >
      {{ option }}
    </v-btn>
  </v-btn-toggle>
</template>

<style scoped>
.range-toggle {
  height: 30px;
}
</style>
