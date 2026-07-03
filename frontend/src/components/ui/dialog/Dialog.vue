<script setup lang="ts">
// Thin wrapper over v-dialog (P3): scrim, Esc-to-close, focus trap and
// focus return come from Vuetify's overlay. Same `open`/`update:open` API.
defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'update:open', value: boolean): void }>()
</script>

<template>
  <v-dialog
    :model-value="open"
    max-width="512"
    @update:model-value="emit('update:open', $event)"
  >
    <v-card class="pa-6">
      <slot />
    </v-card>
  </v-dialog>
</template>
