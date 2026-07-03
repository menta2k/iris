import type { ThemeDefinition } from 'vuetify'

// Vuexy palette (hex tokens lifted from the reference template, see
// docs/vuetify-migration-plan.md — "Target palette").
export const staticPrimaryColor = '#7367F0'
export const staticPrimaryDarkenColor = '#675DD8'

const sharedColors = {
  'primary': staticPrimaryColor,
  'on-primary': '#fff',
  'primary-darken-1': staticPrimaryDarkenColor,
  'secondary': '#808390',
  'on-secondary': '#fff',
  'secondary-darken-1': '#737682',
  'success': '#28C76F',
  'on-success': '#fff',
  'success-darken-1': '#24B364',
  'info': '#00BAD1',
  'on-info': '#fff',
  'info-darken-1': '#00A7BC',
  'warning': '#FF9F43',
  'on-warning': '#fff',
  'warning-darken-1': '#E68F3C',
  'error': '#FF4C51',
  'on-error': '#fff',
  'error-darken-1': '#E64449',
} as const

const sharedVariables = {
  'hover-opacity': 0.06,
  'focus-opacity': 0.1,
  'selected-opacity': 0.08,
  'activated-opacity': 0.16,
  'pressed-opacity': 0.14,
  'dragged-opacity': 0.1,
  'disabled-opacity': 0.4,
  'border-opacity': 0.12,
  'high-emphasis-opacity': 0.9,
  'medium-emphasis-opacity': 0.7,
} as const

export const light: ThemeDefinition = {
  dark: false,
  colors: {
    ...sharedColors,
    'background': '#F8F7FA',
    'on-background': '#2F2B3D',
    'surface': '#fff',
    'on-surface': '#2F2B3D',
    // Subtle inset panels (code blocks, secondary boxes) — replaces the old
    // Tailwind `bg-muted`.
    'surface-light': '#F4F3F6',
  },
  variables: {
    ...sharedVariables,
    'border-color': '#2F2B3D',
    'overlay-scrim-background': '#2F2B3D',
    'overlay-scrim-opacity': 0.5,
    'table-header-color': '#EAEAEC',
    'shadow-key-umbra-color': '#2F2B3D',
  },
}

export const dark: ThemeDefinition = {
  dark: true,
  colors: {
    ...sharedColors,
    'background': '#25293C',
    'on-background': '#E1DEF5',
    'surface': '#2F3349',
    'on-surface': '#E1DEF5',
    'surface-light': '#353A52',
  },
  variables: {
    ...sharedVariables,
    'border-color': '#E1DEF5',
    'overlay-scrim-background': '#171925',
    'overlay-scrim-opacity': 0.6,
    'table-header-color': '#535876',
    'shadow-key-umbra-color': '#131120',
  },
}

export const themes: Record<string, ThemeDefinition> = { light, dark }
