<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  label: string
  value: number // percent 0..100
  threshold?: number // alert threshold; 0/undefined = no threshold coloring
  detail?: string // e.g. "12.3 / 40 GB"
}>()

const pct = computed(() => Math.max(0, Math.min(100, props.value)))

// Colour by proximity to the configured threshold (or a sensible default when
// none is set): at/over threshold = error, near it = warning, else ok.
const tone = computed(() => {
  const t = props.threshold && props.threshold > 0 ? props.threshold : 90
  if (pct.value >= t) return 'error'
  if (pct.value >= t * 0.85) return 'warning'
  return 'success'
})
</script>

<template>
  <div class="usage-meter">
    <div class="d-flex align-center justify-space-between mb-1">
      <span class="text-body-2 font-weight-medium">{{ label }}</span>
      <span class="text-body-2 tabular-nums" :class="`text-${tone}`">{{ pct.toFixed(1) }}%</span>
    </div>
    <div class="track">
      <div class="fill" :class="`bg-${tone}`" :style="{ width: `${pct}%` }" />
      <div
        v-if="threshold && threshold > 0 && threshold < 100"
        class="tick"
        :style="{ left: `${threshold}%` }"
        :title="`Alert threshold ${threshold}%`"
      />
    </div>
    <div v-if="detail || threshold" class="mt-1 d-flex justify-space-between text-caption text-medium-emphasis">
      <span>{{ detail || '' }}</span>
      <span v-if="threshold && threshold > 0">threshold {{ threshold }}%</span>
    </div>
  </div>
</template>

<style scoped>
.track {
  position: relative;
  height: 8px;
  border-radius: 999px;
  background: rgba(var(--v-theme-on-surface), 0.1);
  overflow: hidden;
}
/* Marks where the alert threshold sits on the bar. */
.tick {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 2px;
  background: rgba(var(--v-theme-on-surface), 0.45);
}
.fill {
  height: 100%;
  border-radius: 999px;
  transition: width 0.4s ease;
}
</style>
