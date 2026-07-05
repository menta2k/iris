import { ref, watch } from 'vue'
import { defineStore } from 'pinia'

export type ThemeMode = 'light' | 'dark' | 'system'
export type Skin = 'default' | 'bordered'

export const DEFAULT_PRIMARY = '#7367F0'
const DEFAULT_MODE: ThemeMode = 'dark'
const DEFAULT_SKIN: Skin = 'default'

const STORAGE_KEY = 'iris_ui_config'

export type TableDensity = 'default' | 'compact'

export interface TablePrefs {
  /** Column keys hidden by the user (column-visibility menu). */
  hidden?: string[]
  density?: TableDensity
}

interface PersistedConfig {
  themeMode?: ThemeMode
  primaryColor?: string
  skin?: Skin
  /** Per-table display preferences, keyed by a stable table id. */
  tablePrefs?: Record<string, TablePrefs>
}

function loadPersisted(): PersistedConfig {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== 'object' || parsed === null) return {}
    return parsed as PersistedConfig
  } catch {
    return {}
  }
}

function isThemeMode(v: unknown): v is ThemeMode {
  return v === 'light' || v === 'dark' || v === 'system'
}

function isSkin(v: unknown): v is Skin {
  return v === 'default' || v === 'bordered'
}

function isHexColor(v: unknown): v is string {
  return typeof v === 'string' && /^#[0-9a-fA-F]{6}$/.test(v)
}

// UI preferences behind the settings drawer ("customizer"). Precedence per
// docs/vuetify-migration-plan.md P2: stored value → config default.
export const useConfigStore = defineStore('config', () => {
  const persisted = loadPersisted()

  const themeMode = ref<ThemeMode>(
    isThemeMode(persisted.themeMode) ? persisted.themeMode : DEFAULT_MODE,
  )
  const primaryColor = ref<string>(
    isHexColor(persisted.primaryColor) ? persisted.primaryColor : DEFAULT_PRIMARY,
  )
  const skin = ref<Skin>(isSkin(persisted.skin) ? persisted.skin : DEFAULT_SKIN)
  const tablePrefs = ref<Record<string, TablePrefs>>(
    typeof persisted.tablePrefs === 'object' && persisted.tablePrefs !== null
      ? persisted.tablePrefs
      : {},
  )

  function setTablePrefs(table: string, prefs: TablePrefs) {
    tablePrefs.value = {
      ...tablePrefs.value,
      [table]: { ...tablePrefs.value[table], ...prefs },
    }
  }

  watch([themeMode, primaryColor, skin, tablePrefs], () => {
    const snapshot: PersistedConfig = {
      themeMode: themeMode.value,
      primaryColor: primaryColor.value,
      skin: skin.value,
      tablePrefs: tablePrefs.value,
    }
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(snapshot))
    } catch {
      // Storage unavailable (private mode); preferences stay session-only.
    }
  })

  function reset() {
    themeMode.value = DEFAULT_MODE
    primaryColor.value = DEFAULT_PRIMARY
    skin.value = DEFAULT_SKIN
  }

  return { themeMode, primaryColor, skin, tablePrefs, setTablePrefs, reset }
})
