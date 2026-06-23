import { ref } from 'vue'

// Timezone preference for rendering timestamps. The backend returns UTC ISO
// strings; the UI formats them in the chosen zone. 'system' uses the browser's
// resolved zone. The preference is a module-level singleton persisted to
// localStorage, so every component shares it and reacts to changes.

const STORAGE_KEY = 'iris.timezone'

export const SYSTEM_TIMEZONE = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'

function initial(): string {
  try {
    return localStorage.getItem(STORAGE_KEY) || 'system'
  } catch {
    return 'system'
  }
}

const timezone = ref<string>(initial())

function effectiveZone(): string {
  return timezone.value === 'system' ? SYSTEM_TIMEZONE : timezone.value
}

export function setTimezone(tz: string): void {
  timezone.value = tz
  try {
    localStorage.setItem(STORAGE_KEY, tz)
  } catch {
    /* ignore storage failures (private mode) */
  }
}

/** All IANA zones the runtime knows, or [] when unsupported (older browsers). */
export function listTimezones(): string[] {
  const supported = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] })
    .supportedValuesOf
  return typeof supported === 'function' ? supported('timeZone') : []
}

/**
 * Format a UTC ISO timestamp (or Date) in the selected zone. Reads the timezone
 * ref, so calling it in a template re-renders when the preference changes.
 * Returns '' for empty input and the raw value if it is not a valid date.
 */
export function formatDateTime(value?: string | number | Date | null): string {
  if (value === null || value === undefined || value === '') return ''
  const d = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(d.getTime())) return String(value)
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZone: effectiveZone(),
    timeZoneName: 'short',
  }).format(d)
}

export function useTimezone() {
  return { timezone, systemTimezone: SYSTEM_TIMEZONE, setTimezone, listTimezones, formatDateTime }
}
