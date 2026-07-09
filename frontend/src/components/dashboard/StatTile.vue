<script setup lang="ts">
import { Card } from '@/components/ui/card'

// KPI tile: label, headline value, supporting caption, and a tonal icon
// avatar that carries the metric's semantic color.
defineProps<{
  label: string
  value: string
  icon: string
  color?: string
  caption?: string
  valueClass?: string
}>()
</script>

<template>
  <Card class="h-100">
    <div class="d-flex align-center justify-space-between ga-3 pa-4 h-100">
      <div class="d-flex flex-column ga-1" style="min-width: 0">
        <span class="text-body-2 text-medium-emphasis">{{ label }}</span>
        <span class="text-h5 font-weight-bold tabular-nums text-truncate" :class="valueClass">
          {{ value }}
        </span>
        <span v-if="caption" class="text-caption text-disabled">{{ caption }}</span>
      </div>
      <v-avatar :color="color ?? 'primary'" variant="tonal" rounded size="42">
        <v-icon :icon="icon" size="24" />
      </v-avatar>
    </div>
  </Card>
</template>
