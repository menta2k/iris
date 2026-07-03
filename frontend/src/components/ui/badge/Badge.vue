<script setup lang="ts">
import { computed } from 'vue'

// Thin wrapper mapping the legacy badge API onto v-chip (P3). Tonal chips
// reproduce the old soft "color/15 background + colored text" look.
type Variant = 'default' | 'secondary' | 'destructive' | 'success' | 'warning' | 'outline'

const props = withDefaults(defineProps<{ variant?: Variant; class?: string }>(), {
  variant: 'default',
})

const APPEARANCE: Record<Variant, { color?: string; variant: 'tonal' | 'outlined' }> = {
  default: { color: 'primary', variant: 'tonal' },
  secondary: { color: 'secondary', variant: 'tonal' },
  destructive: { color: 'error', variant: 'tonal' },
  success: { color: 'success', variant: 'tonal' },
  warning: { color: 'warning', variant: 'tonal' },
  outline: { variant: 'outlined' },
}

const appearance = computed(() => APPEARANCE[props.variant])
</script>

<template>
  <v-chip
    :color="appearance.color"
    :variant="appearance.variant"
    size="small"
    label
    class="ui-badge"
    :class="$props.class"
  >
    <slot />
  </v-chip>
</template>

<style scoped>
/* Shared minimum width + centered label so status/MFA/role badges align into
   uniform pills within a column (Active, Disabled, Optional, Required, …).
   Longer multi-word labels expand past the floor as needed. */
.ui-badge {
  min-width: 92px;
}
.ui-badge :deep(.v-chip__content) {
  justify-content: center;
  width: 100%;
}
</style>
