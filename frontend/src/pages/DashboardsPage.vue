<script setup lang="ts">
import { createApp, h, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch, type App } from 'vue'
import { GridStack, type GridStackNode } from 'gridstack'
import 'gridstack/dist/gridstack.min.css'
import PageHeader from '@/components/common/PageHeader.vue'
import MetricWidget from '@/components/dashboard/MetricWidget.vue'
import AddWidgetDialog from '@/components/dashboard/AddWidgetDialog.vue'
import RangeToggle from '@/components/dashboard/RangeToggle.vue'
import { vuetify } from '@/plugins/vuetify'
import { useEventStream } from '@/composables/useEventStream'
import { useToast } from '@/composables/useToast'
import { dashboardsService } from '@/services/dashboards'
import { ApiError } from '@/services/http'
import type { UserDashboard, WidgetConfig } from '@/types'

const { toast } = useToast()

// --- Grid + per-cell Vue apps ---
const gridEl = ref<HTMLDivElement | null>(null)
const grid = shallowRef<GridStack | null>(null)
// Each widget cell hosts its own standalone Vue app so gridstack can own the
// grid DOM without fighting the page's virtual DOM. `state` is reactive so we
// can push config updates / refresh ticks into the mounted MetricWidget.
interface CellEntry {
  app: App
  state: { config: WidgetConfig; refreshKey: number; rangeOverride: WidgetConfig['range'] }
  el: HTMLElement
}
const cells = new Map<string, CellEntry>()

// --- Page state ---
const dashboards = ref<UserDashboard[]>([])
const activeId = ref<string>('')
const widgets = ref<WidgetConfig[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const notImplemented = ref(false)
const editMode = ref(false)
const addOpen = ref(false)

// Dashboard-wide time range: a single toggle that drives every chart, like the
// Overview. It overrides each widget's stored range for display.
const RANGE_OPTIONS: WidgetConfig['range'][] = ['1h', '6h', '24h', '7d']
const dashboardRange = ref<WidgetConfig['range']>('6h')
// Push the selected range into every mounted cell so all charts reload together.
watch(dashboardRange, (r) => {
  for (const entry of cells.values()) entry.state.rangeOverride = r
})

// Widget-edit dialog (title only; range is dashboard-wide).
const editOpen = ref(false)
const editingId = ref<string>('')
const editTitle = ref('')

// New/rename dashboard dialog.
const nameDialogOpen = ref(false)
const nameDialogMode = ref<'new' | 'rename'>('new')
const nameDialogValue = ref('')

const activeDashboard = () => dashboards.value.find((d) => d.id === activeId.value) ?? null

function genId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  return `w-${Date.now()}-${Math.floor(Math.random() * 1e6)}`
}

function parseWidgets(json: string): WidgetConfig[] {
  try {
    const arr = JSON.parse(json || '[]')
    return Array.isArray(arr) ? (arr as WidgetConfig[]) : []
  } catch {
    return []
  }
}

// -- Grid lifecycle ---------------------------------------------------------

function mountCell(cfg: WidgetConfig) {
  const g = grid.value
  if (!g) return
  const el = g.addWidget({ x: cfg.x, y: cfg.y, w: cfg.w, h: cfg.h, id: cfg.id })
  const content = el.querySelector('.grid-stack-item-content') as HTMLElement | null
  if (!content) return
  const state = reactive({ config: cfg, refreshKey: 0, rangeOverride: dashboardRange.value }) as CellEntry['state']
  const app = createApp({
    render: () =>
      h(MetricWidget, {
        config: state.config,
        refreshKey: state.refreshKey,
        rangeOverride: state.rangeOverride,
        onEdit: () => openEditWidget(cfg.id),
        onRemove: () => removeWidget(cfg.id),
      }),
  })
  app.use(vuetify)
  app.mount(content)
  cells.set(cfg.id, { app, state, el })
}

function unmountCell(id: string, removeDom = true) {
  const entry = cells.get(id)
  if (!entry) return
  entry.app.unmount()
  cells.delete(id)
  if (removeDom && grid.value) grid.value.removeWidget(entry.el, true)
}

function clearGrid() {
  for (const id of [...cells.keys()]) unmountCell(id, false)
  grid.value?.removeAll(true)
}

function renderWidgets() {
  const g = grid.value
  if (!g) return
  g.batchUpdate()
  clearGrid()
  for (const cfg of widgets.value) mountCell(cfg)
  g.batchUpdate(false)
}

// -- Persistence ------------------------------------------------------------

let saveTimer: ReturnType<typeof setTimeout> | undefined

function syncGeometryFromGrid() {
  const g = grid.value
  if (!g) return
  const saved = g.save(false) as GridStackNode[]
  const byId = new Map(saved.map((n) => [String(n.id), n]))
  widgets.value = widgets.value.map((w) => {
    const n = byId.get(w.id)
    if (!n) return w
    return { ...w, x: n.x ?? w.x, y: n.y ?? w.y, w: n.w ?? w.w, h: n.h ?? w.h }
  })
}

function scheduleSave() {
  clearTimeout(saveTimer)
  saveTimer = setTimeout(persist, 800)
}

async function persist() {
  const dash = activeDashboard()
  if (!dash) return
  try {
    const updated = await dashboardsService.update(dash.id, {
      name: dash.name,
      widgetsJson: JSON.stringify(widgets.value),
    })
    dashboards.value = dashboards.value.map((d) => (d.id === updated.id ? updated : d))
  } catch (err) {
    toast({
      title: 'Save failed',
      description: err instanceof Error ? err.message : 'Could not save the dashboard layout.',
      variant: 'destructive',
    })
  }
}

function onGridChange() {
  syncGeometryFromGrid()
  scheduleSave()
}

// -- Widget CRUD ------------------------------------------------------------

function addWidget(partial: Omit<WidgetConfig, 'id' | 'x' | 'y' | 'w' | 'h' | 'range'>) {
  const isStat = partial.viz === 'stat' || partial.viz === 'gauge'
  const cfg: WidgetConfig = {
    ...partial,
    id: genId(),
    x: 0,
    y: 0, // gridstack finds an open slot via float/compact
    w: isStat ? 3 : 6,
    h: isStat ? 3 : 4,
    // New widgets adopt the dashboard-wide range; the header toggle drives it.
    range: dashboardRange.value,
  }
  widgets.value = [...widgets.value, cfg]
  mountCell(cfg)
  syncGeometryFromGrid()
  scheduleSave()
}

function removeWidget(id: string) {
  unmountCell(id)
  widgets.value = widgets.value.filter((w) => w.id !== id)
  scheduleSave()
}

function openEditWidget(id: string) {
  const w = widgets.value.find((x) => x.id === id)
  if (!w) return
  editingId.value = id
  editTitle.value = w.title
  editOpen.value = true
}

function applyEditWidget() {
  const id = editingId.value
  const idx = widgets.value.findIndex((w) => w.id === id)
  if (idx === -1) return
  const next = { ...widgets.value[idx], title: editTitle.value.trim() || widgets.value[idx].title }
  widgets.value = widgets.value.map((w) => (w.id === id ? next : w))
  // Push the new config into the mounted cell reactively.
  const entry = cells.get(id)
  if (entry) entry.state.config = next
  editOpen.value = false
  scheduleSave()
}

// -- Dashboard CRUD ---------------------------------------------------------

async function loadDashboards() {
  loading.value = true
  error.value = null
  notImplemented.value = false
  try {
    const list = await dashboardsService.list()
    dashboards.value = list
    if (list.length > 0) {
      const def = list.find((d) => d.isDefault) ?? list[0]
      selectDashboard(def.id)
    } else {
      activeId.value = ''
      widgets.value = []
    }
  } catch (err) {
    if (err instanceof ApiError && err.notImplemented) {
      notImplemented.value = true
    } else {
      error.value = err instanceof Error ? err.message : 'Failed to load dashboards.'
    }
  } finally {
    loading.value = false
  }
}

function selectDashboard(id: string) {
  const dash = dashboards.value.find((d) => d.id === id)
  if (!dash) return
  activeId.value = id
  widgets.value = parseWidgets(dash.widgetsJson)
  // Defer to next tick so the grid element exists after any v-if swap.
  requestAnimationFrame(() => renderWidgets())
}

function openNewDashboard() {
  nameDialogMode.value = 'new'
  nameDialogValue.value = ''
  nameDialogOpen.value = true
}

function openRenameDashboard() {
  const dash = activeDashboard()
  if (!dash) return
  nameDialogMode.value = 'rename'
  nameDialogValue.value = dash.name
  nameDialogOpen.value = true
}

async function applyNameDialog() {
  const name = nameDialogValue.value.trim()
  if (!name) return
  try {
    if (nameDialogMode.value === 'new') {
      const created = await dashboardsService.create({
        name,
        widgetsJson: '[]',
        makeDefault: dashboards.value.length === 0,
      })
      dashboards.value = [...dashboards.value, created]
      nameDialogOpen.value = false
      selectDashboard(created.id)
    } else {
      const dash = activeDashboard()
      if (!dash) return
      const updated = await dashboardsService.update(dash.id, {
        name,
        widgetsJson: JSON.stringify(widgets.value),
      })
      dashboards.value = dashboards.value.map((d) => (d.id === updated.id ? updated : d))
      nameDialogOpen.value = false
    }
  } catch (err) {
    toast({
      title: 'Save failed',
      description: err instanceof Error ? err.message : 'Could not save the dashboard.',
      variant: 'destructive',
    })
  }
}

async function deleteDashboard() {
  const dash = activeDashboard()
  if (!dash) return
  if (!window.confirm(`Delete dashboard "${dash.name}"? This cannot be undone.`)) return
  try {
    await dashboardsService.remove(dash.id)
    dashboards.value = dashboards.value.filter((d) => d.id !== dash.id)
    if (dashboards.value.length > 0) {
      const def = dashboards.value.find((d) => d.isDefault) ?? dashboards.value[0]
      selectDashboard(def.id)
    } else {
      clearGrid()
      activeId.value = ''
      widgets.value = []
    }
    toast({ title: 'Dashboard deleted', variant: 'success' })
  } catch (err) {
    toast({
      title: 'Delete failed',
      description: err instanceof Error ? err.message : 'Could not delete the dashboard.',
      variant: 'destructive',
    })
  }
}

async function setDefault() {
  const dash = activeDashboard()
  if (!dash || dash.isDefault) return
  try {
    const updated = await dashboardsService.setDefault(dash.id)
    dashboards.value = dashboards.value.map((d) => ({ ...d, isDefault: d.id === updated.id }))
    toast({ title: `"${updated.name}" is now your default`, variant: 'success' })
  } catch (err) {
    toast({
      title: 'Failed',
      description: err instanceof Error ? err.message : 'Could not set default.',
      variant: 'destructive',
    })
  }
}

function toggleEdit() {
  editMode.value = !editMode.value
  grid.value?.setStatic(!editMode.value)
}

// -- Refresh (auto: SSE tick + interval; gated by the Auto-refresh toggle) --

const REFRESH_MS = 60_000
const autoRefresh = ref(true)

function refreshAll() {
  for (const entry of cells.values()) entry.state.refreshKey += 1
}
let refreshTimer: ReturnType<typeof setInterval> | undefined
const dashStream = useEventStream('dashboard', () => {
  if (autoRefresh.value) refreshAll()
})

// Start/stop the polling interval and live SSE tick with the toggle.
watch(
  autoRefresh,
  (on) => {
    clearInterval(refreshTimer)
    if (on) {
      refreshTimer = setInterval(refreshAll, REFRESH_MS)
      dashStream.start()
    } else {
      dashStream.stop()
    }
  },
  { immediate: true },
)

onMounted(() => {
  if (gridEl.value) {
    grid.value = GridStack.init(
      {
        column: 12,
        cellHeight: 68,
        margin: 8,
        float: true,
        staticGrid: true, // starts in view mode; Edit toggle enables drag/resize
        handleClass: 'metric-widget__header',
      },
      gridEl.value,
    )
    grid.value.on('change', onGridChange)
  }
  loadDashboards()
})

onBeforeUnmount(() => {
  clearTimeout(saveTimer)
  clearInterval(refreshTimer)
  dashStream.stop()
  clearGrid()
  grid.value?.destroy(false)
  grid.value = null
})
</script>

<template>
  <div>
    <PageHeader title="Dashboards" description="Build your own metric dashboards from the catalog or raw PromQL.">
      <template #actions>
        <v-select
          v-if="dashboards.length > 0"
          :model-value="activeId"
          :items="dashboards.map((d) => ({ title: d.isDefault ? `${d.name} ★` : d.name, value: d.id }))"
          density="compact"
          hide-details
          variant="outlined"
          style="min-width: 200px"
          @update:model-value="selectDashboard"
        />
        <RangeToggle v-if="activeId" v-model="dashboardRange" :options="RANGE_OPTIONS" />
        <v-switch
          v-if="activeId"
          v-model="autoRefresh"
          label="Auto-refresh"
          color="primary"
          density="compact"
          hide-details
          class="flex-grow-0"
        />
        <v-btn
          v-if="activeId"
          icon="mdi-refresh"
          variant="text"
          size="small"
          aria-label="Refresh now"
          @click="refreshAll"
        />
        <v-btn
          v-if="activeId"
          :color="editMode ? 'primary' : undefined"
          :variant="editMode ? 'flat' : 'tonal'"
          prepend-icon="mdi-view-dashboard-edit-outline"
          @click="toggleEdit"
        >
          {{ editMode ? 'Done' : 'Edit layout' }}
        </v-btn>
        <v-btn
          v-if="activeId"
          color="primary"
          prepend-icon="mdi-plus"
          @click="addOpen = true"
        >
          Add widget
        </v-btn>
        <v-menu v-if="activeId">
          <template #activator="{ props }">
            <v-btn icon="mdi-dots-vertical" variant="text" v-bind="props" aria-label="Dashboard actions" />
          </template>
          <v-list density="compact">
            <v-list-item prepend-icon="mdi-plus-box-outline" title="New dashboard" @click="openNewDashboard" />
            <v-list-item prepend-icon="mdi-rename-outline" title="Rename" @click="openRenameDashboard" />
            <v-list-item prepend-icon="mdi-star-outline" title="Set as default" @click="setDefault" />
            <v-divider />
            <v-list-item
              prepend-icon="mdi-delete-outline"
              title="Delete"
              base-color="error"
              @click="deleteDashboard"
            />
          </v-list>
        </v-menu>
      </template>
    </PageHeader>

    <!-- States -->
    <v-alert v-if="notImplemented" type="info" variant="tonal" class="mb-4">
      The dashboards API is not available on this backend yet.
    </v-alert>
    <v-alert v-else-if="error" type="error" variant="tonal" class="mb-4">{{ error }}</v-alert>
    <div v-else-if="loading" class="text-medium-emphasis text-body-2 py-8 text-center">Loading…</div>

    <!-- Empty state -->
    <v-card
      v-else-if="dashboards.length === 0"
      variant="tonal"
      class="pa-8 text-center"
      data-testid="dashboards-empty"
    >
      <v-icon icon="mdi-view-grid-plus-outline" size="48" class="mb-3 text-medium-emphasis" />
      <div class="text-h6 mb-1">No dashboards yet</div>
      <p class="text-body-2 text-medium-emphasis mb-4">
        Create your first dashboard and add metric widgets from the catalog.
      </p>
      <v-btn color="primary" prepend-icon="mdi-plus" @click="openNewDashboard">Create dashboard</v-btn>
    </v-card>

    <!-- Grid (kept mounted so gridstack keeps its DOM; hidden while empty).
         The edit-mode class lives on THIS wrapper, never on the .grid-stack
         element itself: gridstack imperatively adds a `gs-id-N` class to that
         element (its injected height CSS is scoped to it), and a Vue :class
         binding on the same element would clobber `gs-id-N` on every toggle,
         collapsing all items to 0px height. -->
    <div
      v-show="dashboards.length > 0 && !loading && !error && !notImplemented"
      :class="{ 'grid-stack--editing': editMode }"
    >
      <div
        v-if="activeId && widgets.length === 0"
        class="text-center text-medium-emphasis text-body-2 py-8"
      >
        This dashboard is empty. Click <strong>Add widget</strong> to get started.
      </div>
      <div ref="gridEl" class="grid-stack" />
    </div>

    <AddWidgetDialog v-model="addOpen" @add="addWidget" />

    <!-- Edit widget dialog -->
    <v-dialog v-model="editOpen" max-width="420">
      <v-card>
        <v-card-title>Edit widget</v-card-title>
        <v-card-text>
          <v-text-field v-model="editTitle" label="Title" density="compact" hide-details class="mb-1" />
          <p class="text-caption text-medium-emphasis mt-2 mb-0">
            The time range is controlled for the whole dashboard by the range toggle in the header.
          </p>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="editOpen = false">Cancel</v-btn>
          <v-btn color="primary" variant="flat" @click="applyEditWidget">Save</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- New / rename dialog -->
    <v-dialog v-model="nameDialogOpen" max-width="420">
      <v-card>
        <v-card-title>{{ nameDialogMode === 'new' ? 'New dashboard' : 'Rename dashboard' }}</v-card-title>
        <v-card-text>
          <v-text-field
            v-model="nameDialogValue"
            label="Name"
            density="compact"
            hide-details
            autofocus
            @keyup.enter="applyNameDialog"
          />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="nameDialogOpen = false">Cancel</v-btn>
          <v-btn color="primary" variant="flat" :disabled="!nameDialogValue.trim()" @click="applyNameDialog">
            {{ nameDialogMode === 'new' ? 'Create' : 'Save' }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<style scoped>
/* Give edit mode an affordance: a dashed outline around draggable cells. */
.grid-stack--editing :deep(.grid-stack-item-content) {
  outline: 1px dashed rgba(var(--v-theme-primary), 0.4);
}
.grid-stack--editing :deep(.metric-widget__header) {
  cursor: move;
}
</style>
