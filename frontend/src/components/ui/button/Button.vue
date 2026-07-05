<script setup lang="ts">
import { computed } from 'vue'

// Thin wrapper mapping the legacy shadcn-style API onto v-btn so ~30 call
// sites keep working unchanged during the Vuetify migration (P3).
type Variant = 'default' | 'destructive' | 'outline' | 'secondary' | 'ghost' | 'link'
type Size = 'default' | 'sm' | 'lg' | 'icon'

const props = withDefaults(
  defineProps<{
    variant?: Variant
    size?: Size
    type?: 'button' | 'submit' | 'reset'
    disabled?: boolean
    class?: string
  }>(),
  { variant: 'default', size: 'default', type: 'button' },
)

const APPEARANCE: Record<
  Variant,
  { color?: string; variant: 'flat' | 'outlined' | 'tonal' | 'text' | 'plain' }
> = {
  default: { color: 'primary', variant: 'flat' },
  destructive: { color: 'error', variant: 'flat' },
  outline: { variant: 'outlined' },
  secondary: { color: 'secondary', variant: 'tonal' },
  ghost: { variant: 'text' },
  link: { color: 'primary', variant: 'plain' },
}

const SIZES: Record<Size, string | undefined> = {
  default: undefined,
  sm: 'small',
  lg: 'large',
  icon: 'small',
}

const appearance = computed(() => APPEARANCE[props.variant])
const vSize = computed(() => SIZES[props.size])
</script>

<template>
  <v-btn
    :type="type"
    :disabled="disabled"
    :color="appearance.color"
    :variant="appearance.variant"
    :size="vSize"
    :icon="size === 'icon'"
    :class="$props.class"
  >
    <slot />
  </v-btn>
</template>
