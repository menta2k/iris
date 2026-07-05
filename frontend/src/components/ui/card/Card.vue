<script setup lang="ts">
// Thin wrapper over v-card (P3). Header/Title/Description/Content stay
// presentational divs; the surface (color, radius, elevation, bordered
// skin) now comes from Vuetify.
defineProps<{ class?: string }>()
</script>

<template>
  <v-card :class="$props.class">
    <slot />
  </v-card>
</template>
