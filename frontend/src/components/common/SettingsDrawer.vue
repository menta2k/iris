<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useConfigStore, DEFAULT_PRIMARY, type ThemeMode, type Skin } from '@/stores/config'

const open = defineModel<boolean>({ default: false })

const store = useConfigStore()
const { themeMode, primaryColor, skin } = storeToRefs(store)

const modes: Array<{ value: ThemeMode; label: string; icon: string }> = [
  { value: 'light', label: 'Light', icon: 'mdi-weather-sunny' },
  { value: 'dark', label: 'Dark', icon: 'mdi-weather-night' },
  { value: 'system', label: 'System', icon: 'mdi-monitor' },
]

// Vuexy customizer accent presets.
const presetColors = [
  DEFAULT_PRIMARY,
  '#0D9394',
  '#FFB400',
  '#FF4C51',
  '#16B1FF',
  '#28C76F',
]

const skins: Array<{ value: Skin; label: string }> = [
  { value: 'default', label: 'Default' },
  { value: 'bordered', label: 'Bordered' },
]
</script>

<template>
  <v-navigation-drawer
    v-model="open"
    location="right"
    temporary
    width="320"
    aria-label="Theme settings"
  >
    <div class="d-flex align-center justify-space-between px-4 py-3">
      <div>
        <p class="text-body-2 font-weight-bold">Theme Customizer</p>
        <p class="text-caption text-medium-emphasis">Customize and preview in real time</p>
      </div>
      <div class="d-flex align-center ga-1">
        <v-btn
          icon="mdi-refresh"
          variant="text"
          size="small"
          aria-label="Reset to defaults"
          @click="store.reset()"
        />
        <v-btn
          icon="mdi-close"
          variant="text"
          size="small"
          aria-label="Close settings"
          @click="open = false"
        />
      </div>
    </div>
    <v-divider />

    <div class="px-4 py-4">
      <p class="pb-2 text-caption font-weight-bold text-uppercase text-medium-emphasis">
        Mode
      </p>
      <v-btn-toggle v-model="themeMode" mandatory divided density="comfortable" class="w-100">
        <v-btn
          v-for="mode in modes"
          :key="mode.value"
          :value="mode.value"
          :prepend-icon="mode.icon"
          class="flex-grow-1"
          size="small"
        >
          {{ mode.label }}
        </v-btn>
      </v-btn-toggle>
    </div>

    <div class="px-4 pb-4">
      <p class="pb-2 text-caption font-weight-bold text-uppercase text-medium-emphasis">
        Primary color
      </p>
      <div class="d-flex flex-wrap ga-2">
        <button
          v-for="color in presetColors"
          :key="color"
          type="button"
          class="swatch rounded"
          :class="primaryColor === color ? 'swatch--selected' : ''"
          :style="{ backgroundColor: color }"
          :aria-label="`Set primary color ${color}`"
          :aria-pressed="primaryColor === color"
          @click="primaryColor = color"
        />
      </div>
    </div>

    <div class="px-4 pb-4">
      <p class="pb-2 text-caption font-weight-bold text-uppercase text-medium-emphasis">
        Skin
      </p>
      <v-btn-toggle v-model="skin" mandatory divided density="comfortable" class="w-100">
        <v-btn v-for="s in skins" :key="s.value" :value="s.value" class="flex-grow-1" size="small">
          {{ s.label }}
        </v-btn>
      </v-btn-toggle>
    </div>
  </v-navigation-drawer>
</template>

<style scoped>
.swatch {
  width: 36px;
  height: 36px;
  border: none;
  cursor: pointer;
  transition: transform 0.15s ease;
}
.swatch--selected {
  transform: scale(1.1);
  outline: 2px solid rgb(var(--v-theme-primary));
  outline-offset: 2px;
}
</style>
