<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { metricsService } from '@/services/metrics'
import { ApiError } from '@/services/http'
import { panelRegistry } from './panels'
import type { WidgetCatalogEntry, WidgetConfig, WidgetSource, WidgetViz } from '@/types'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  // A widget config WITHOUT geometry/id/range — the page assigns x/y/w/h/id and
  // the dashboard-wide range.
  (e: 'add', widget: Omit<WidgetConfig, 'id' | 'x' | 'y' | 'w' | 'h' | 'range'>): void
}>()

const tab = ref<WidgetSource>('catalog')

// --- Catalog tab ---
const catalog = ref<WidgetCatalogEntry[]>([])
const catalogLoading = ref(false)
const catalogError = ref<string | null>(null)
const selectedKey = ref<string>('')
const catalogGroupBy = ref<string>('')

const grouped = computed(() => {
  const by = new Map<string, WidgetCatalogEntry[]>()
  for (const w of catalog.value) {
    const list = by.get(w.category) ?? []
    list.push(w)
    by.set(w.category, list)
  }
  return [...by.entries()].map(([category, widgets]) => ({ category, widgets }))
})

const selectedEntry = computed(() => catalog.value.find((w) => w.key === selectedKey.value) ?? null)

// --- Panels tab (iris data panels, not metrics) ---
const selectedPanel = ref<string>('')
const groupedPanels = computed(() => {
  const by = new Map<string, typeof panelRegistry>()
  for (const p of panelRegistry) {
    const list = by.get(p.category) ?? []
    list.push(p)
    by.set(p.category, list)
  }
  return [...by.entries()].map(([category, panels]) => ({ category, panels }))
})
const selectedPanelDef = computed(() => panelRegistry.find((p) => p.key === selectedPanel.value) ?? null)

async function loadCatalog() {
  catalogLoading.value = true
  catalogError.value = null
  try {
    catalog.value = await metricsService.getWidgetCatalog()
  } catch (err) {
    if (err instanceof ApiError && err.notImplemented) {
      catalogError.value = 'Widget catalog endpoint not available.'
    } else {
      catalogError.value = err instanceof Error ? err.message : 'Failed to load catalog.'
    }
  } finally {
    catalogLoading.value = false
  }
}

// --- Advanced / PromQL tab ---
const promql = ref('')

// --- Common fields ---
const title = ref('')
// Visualization for either source. Defaults from the catalog def when a catalog
// widget is picked; the user can override (e.g. show a grouped metric as a table).
const viz = ref<WidgetViz>('line')
const VIZ_OPTIONS: { title: string; value: WidgetViz }[] = [
  { title: 'Line', value: 'line' },
  { title: 'Area', value: 'area' },
  { title: 'Bar', value: 'bar' },
  { title: 'Table', value: 'table' },
  { title: 'Stat (single value)', value: 'stat' },
]

// When picking a catalog widget, default the title/viz/group-by from its def.
// The time range is dashboard-wide (set via the header toggle), so it is not
// chosen here.
watch(selectedKey, () => {
  const e = selectedEntry.value
  if (!e) return
  if (!title.value.trim()) title.value = e.title
  viz.value = e.viz
  catalogGroupBy.value = ''
})

// Picking a panel defaults the title from its def.
watch(selectedPanel, () => {
  const p = selectedPanelDef.value
  if (p && !title.value.trim()) title.value = p.title
})

const canSubmit = computed(() => {
  if (!title.value.trim()) return false
  if (tab.value === 'catalog') return !!selectedKey.value
  if (tab.value === 'panel') return !!selectedPanel.value
  return promql.value.trim().length > 0
})

function reset() {
  tab.value = 'catalog'
  selectedKey.value = ''
  catalogGroupBy.value = ''
  selectedPanel.value = ''
  promql.value = ''
  viz.value = 'line'
  title.value = ''
}

function close() {
  emit('update:modelValue', false)
}

function submit() {
  if (!canSubmit.value) return
  if (tab.value === 'catalog') {
    const e = selectedEntry.value
    if (!e) return
    emit('add', {
      title: title.value.trim(),
      source: 'catalog',
      catalogKey: e.key,
      viz: viz.value,
      groupBy: e.supportsGroupBy ? catalogGroupBy.value || undefined : undefined,
      unit: e.unit || undefined,
    })
  } else if (tab.value === 'panel') {
    const p = selectedPanelDef.value
    if (!p) return
    emit('add', {
      title: title.value.trim(),
      source: 'panel',
      panelKey: p.key,
      viz: 'table', // panels render their own table; viz is unused
    })
  } else {
    emit('add', {
      title: title.value.trim(),
      source: 'promql',
      promql: promql.value.trim(),
      viz: viz.value,
    })
  }
  close()
}

// Load the catalog the first time the dialog opens; reset the form each open.
watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      reset()
      if (catalog.value.length === 0) loadCatalog()
    }
  },
)
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    max-width="720"
    scrollable
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card>
      <v-card-title class="d-flex align-center justify-space-between">
        <span>Add widget</span>
        <v-btn icon="mdi-close" variant="text" density="comfortable" @click="close" />
      </v-card-title>
      <v-divider />

      <v-tabs v-model="tab" density="comfortable" color="primary">
        <v-tab value="catalog">Catalog</v-tab>
        <v-tab value="panel">Panels</v-tab>
        <v-tab value="promql">Advanced (PromQL)</v-tab>
      </v-tabs>
      <v-divider />

      <v-card-text style="max-height: 60vh">
        <v-window v-model="tab">
          <!-- Catalog -->
          <v-window-item value="catalog">
            <div v-if="catalogLoading" class="text-medium-emphasis text-body-2 py-4">Loading catalog…</div>
            <v-alert v-else-if="catalogError" type="warning" variant="tonal" density="compact" class="my-2">
              {{ catalogError }}
            </v-alert>
            <template v-else>
              <v-expansion-panels v-model="selectedKey" variant="accordion" class="mb-2">
                <template v-for="group in grouped" :key="group.category">
                  <div class="text-overline text-medium-emphasis px-2 pt-2">{{ group.category }}</div>
                  <v-radio-group v-model="selectedKey" hide-details density="compact" class="mb-2">
                    <v-radio
                      v-for="w in group.widgets"
                      :key="w.key"
                      :value="w.key"
                      :label="w.title"
                    >
                      <template #label>
                        <div>
                          <div class="text-body-2">{{ w.title }}</div>
                          <div class="text-caption text-medium-emphasis">{{ w.description }}</div>
                        </div>
                      </template>
                    </v-radio>
                  </v-radio-group>
                </template>
              </v-expansion-panels>

              <v-select
                v-if="selectedEntry?.supportsGroupBy"
                v-model="catalogGroupBy"
                :items="[{ title: '— No grouping —', value: '' }, ...(selectedEntry?.groupByLabels ?? []).map((l) => ({ title: l, value: l }))]"
                label="Group by"
                density="compact"
                hide-details
                class="mb-2"
              />
            </template>
          </v-window-item>

          <!-- Panels (iris data, not metrics) -->
          <v-window-item value="panel">
            <p class="text-caption text-medium-emphasis mb-2">
              Operational panels backed by iris data (not Prometheus).
            </p>
            <template v-for="group in groupedPanels" :key="group.category">
              <div class="text-overline text-medium-emphasis px-2 pt-2">{{ group.category }}</div>
              <v-radio-group v-model="selectedPanel" hide-details density="compact" class="mb-2">
                <v-radio v-for="p in group.panels" :key="p.key" :value="p.key">
                  <template #label>
                    <div>
                      <div class="text-body-2">{{ p.title }}</div>
                      <div class="text-caption text-medium-emphasis">{{ p.description }}</div>
                    </div>
                  </template>
                </v-radio>
              </v-radio-group>
            </template>
          </v-window-item>

          <!-- Advanced PromQL -->
          <v-window-item value="promql">
            <v-textarea
              v-model="promql"
              label="PromQL expression"
              placeholder="sum(rate(total_messages_received[$window]))"
              rows="3"
              auto-grow
              density="compact"
              class="mb-1"
              hint="Use $window for the rate window. Read-only queries only; results are capped at 20 series and 10s."
              persistent-hint
            />
          </v-window-item>
        </v-window>

        <v-divider class="my-3" />

        <div class="d-flex flex-wrap ga-3">
          <v-text-field
            v-model="title"
            label="Widget title"
            density="compact"
            hide-details
            style="min-width: 220px; flex: 1 1 220px"
          />
          <v-select
            v-if="tab !== 'panel'"
            v-model="viz"
            :items="VIZ_OPTIONS"
            label="Display as"
            density="compact"
            hide-details
            style="max-width: 200px"
          />
        </div>
        <p class="text-caption text-medium-emphasis mt-2 mb-0">
          Time range is set for the whole dashboard from the header toggle.
        </p>
      </v-card-text>

      <v-divider />
      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" @click="close">Cancel</v-btn>
        <v-btn color="primary" variant="flat" :disabled="!canSubmit" @click="submit">Add widget</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>
