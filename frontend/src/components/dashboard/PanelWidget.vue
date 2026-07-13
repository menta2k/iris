<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ApiError } from '@/services/http'
import { lookupPanel, type PanelRow } from './panels'
import type { WidgetConfig } from '@/types'

const props = defineProps<{
  config: WidgetConfig
  refreshKey?: number
  rangeOverride?: string
}>()

const emit = defineEmits<{
  (e: 'edit'): void
  (e: 'remove'): void
}>()

const panel = computed(() => (props.config.panelKey ? lookupPanel(props.config.panelKey) : undefined))
const effectiveRange = computed(() => props.rangeOverride || props.config.range)

const loading = ref(false)
const error = ref<string | null>(null)
const notImplemented = ref(false)
const rows = ref<PanelRow[]>([])

async function load() {
  const def = panel.value
  if (!def) return
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    rows.value = await def.load({ range: effectiveRange.value })
  } catch (err) {
    rows.value = []
    if (err instanceof ApiError && err.notImplemented) {
      notImplemented.value = true
    } else {
      error.value = err instanceof Error ? err.message : 'Failed to load panel.'
    }
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(() => [props.config.panelKey, effectiveRange.value, props.refreshKey], load)

const hasData = computed(() => rows.value.length > 0)
</script>

<template>
  <v-card class="metric-widget d-flex flex-column h-100" elevation="1">
    <div class="d-flex align-center justify-space-between px-3 py-2 metric-widget__header">
      <div class="text-body-2 font-weight-medium text-truncate">{{ config.title }}</div>
      <div class="d-flex align-center ga-1">
        <v-btn
          icon="mdi-pencil-outline"
          size="x-small"
          variant="text"
          density="comfortable"
          :aria-label="`Edit ${config.title}`"
          @click="emit('edit')"
        />
        <v-btn
          icon="mdi-close"
          size="x-small"
          variant="text"
          density="comfortable"
          :aria-label="`Remove ${config.title}`"
          @click="emit('remove')"
        />
      </div>
    </div>
    <v-divider />

    <div class="flex-grow-1 position-relative panel-widget__body">
      <table v-if="panel && hasData" class="panel-table">
        <thead>
          <tr>
            <th
              v-for="col in panel.columns"
              :key="col.key"
              :class="{ 'text-right': col.align === 'end' }"
            >
              {{ col.label }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, i) in rows" :key="i">
            <td
              v-for="col in panel.columns"
              :key="col.key"
              :class="{ 'text-right tabular-nums': col.align === 'end' }"
            >
              {{ row[col.key] }}
            </td>
          </tr>
        </tbody>
      </table>

      <div
        v-if="!panel || loading || error || notImplemented || !hasData"
        class="position-absolute top-0 left-0 right-0 bottom-0 d-flex align-center justify-center text-center text-caption text-medium-emphasis pa-3"
      >
        <span v-if="!panel" class="text-error">Unknown panel.</span>
        <span v-else-if="loading">Loading…</span>
        <span v-else-if="error" class="text-error">{{ error }}</span>
        <span v-else-if="notImplemented">This panel is not available on this backend.</span>
        <span v-else>No data yet.</span>
      </div>
    </div>
  </v-card>
</template>

<style scoped>
.metric-widget {
  overflow: hidden;
}
.panel-widget__body {
  overflow: auto;
  min-height: 0;
}
.panel-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.8125rem;
}
.panel-table th,
.panel-table td {
  padding: 5px 10px;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 220px;
}
.panel-table th {
  position: sticky;
  top: 0;
  background: rgb(var(--v-theme-surface));
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.7);
  border-bottom: 1px solid rgba(var(--v-border-color), 0.14);
}
.panel-table td {
  border-bottom: 1px solid rgba(var(--v-border-color), 0.08);
}
.panel-table tr:last-child td {
  border-bottom: none;
}
.text-right {
  text-align: right !important;
}
</style>
