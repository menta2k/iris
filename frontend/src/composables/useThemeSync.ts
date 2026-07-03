import { computed, ref, watchEffect } from 'vue'
import { storeToRefs } from 'pinia'
import { useTheme } from 'vuetify'
import { useConfigStore } from '@/stores/config'

// Applies the config store's theme preferences to Vuetify: active theme name
// (light/dark/system) and the live primary color.
export function useThemeSync(): void {
  const theme = useTheme()
  const { themeMode, primaryColor } = storeToRefs(useConfigStore())

  const systemPrefersDark = window.matchMedia('(prefers-color-scheme: dark)')
  const systemDark = ref(systemPrefersDark.matches)
  systemPrefersDark.addEventListener('change', (e) => {
    systemDark.value = e.matches
  })

  const isDark = computed(() =>
    themeMode.value === 'system' ? systemDark.value : themeMode.value === 'dark',
  )

  watchEffect(() => {
    theme.global.name.value = isDark.value ? 'dark' : 'light'
  })

  // Vuetify's documented runtime-theming API: mutate the reactive theme
  // definition. Applied to both themes so mode switches keep the color.
  watchEffect(() => {
    for (const name of ['light', 'dark'] as const) {
      const colors = theme.themes.value[name]?.colors
      if (colors) colors.primary = primaryColor.value
    }
  })
}
