import { computed, type ComputedRef } from 'vue'
import { useTheme } from 'vuetify'

// Theme-reactive colors for ECharts panels. Charts can't consume CSS variables
// directly, so this resolves the active Vuetify theme into concrete values and
// recomputes when the user switches mode or primary color.
export interface ChartTheme {
  isDark: boolean
  axisLabel: string
  axisLine: string
  splitLine: string
  legendText: string
  tooltipBg: string
  tooltipBorder: string
  tooltipText: string
  // Semantic series colors. Light mode uses the darken-1 steps — the base
  // Vuexy status colors are too light to hold up on a white chart surface
  // (validated with the dataviz palette checker).
  series: {
    success: string
    info: string
    warning: string
    error: string
    primary: string
  }
}

function withAlpha(hex: string, alpha: number): string {
  const h = hex.replace('#', '')
  const full = h.length === 3 ? h.split('').map((c) => c + c).join('') : h
  const r = parseInt(full.slice(0, 2), 16)
  const g = parseInt(full.slice(2, 4), 16)
  const b = parseInt(full.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

export function useChartTheme(): ComputedRef<ChartTheme> {
  const theme = useTheme()

  return computed<ChartTheme>(() => {
    const current = theme.current.value
    const colors = current.colors
    const dark = current.dark
    const onSurface = colors['on-surface'] ?? (dark ? '#E1DEF5' : '#2F2B3D')

    const pick = (name: string): string =>
      dark ? colors[name] : (colors[`${name}-darken-1`] ?? colors[name])

    return {
      isDark: dark,
      axisLabel: withAlpha(onSurface, 0.55),
      axisLine: withAlpha(onSurface, 0.16),
      splitLine: withAlpha(onSurface, dark ? 0.12 : 0.08),
      legendText: withAlpha(onSurface, 0.7),
      tooltipBg: colors.surface ?? (dark ? '#2F3349' : '#fff'),
      tooltipBorder: withAlpha(onSurface, 0.16),
      tooltipText: withAlpha(onSurface, 0.9),
      series: {
        success: pick('success'),
        info: pick('info'),
        warning: pick('warning'),
        error: pick('error'),
        primary: colors.primary,
      },
    }
  })
}
