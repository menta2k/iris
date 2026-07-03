// Time helpers for fixtures. Times are anchored to module-load time so the
// "recent" activity always looks fresh without being frozen to a hard-coded date.

const MIN = 60_000
const HOUR = 60 * MIN
const DAY = 24 * HOUR
const BASE = Date.now()

export function iso(msAgo: number): string {
  return new Date(BASE - msAgo).toISOString()
}

export function minutesAgo(n: number): string {
  return iso(n * MIN)
}

export function hoursAgo(n: number): string {
  return iso(n * HOUR)
}

export function daysAgo(n: number): string {
  return iso(n * DAY)
}

/** Calendar date (YYYY-MM-DD) n days in the past. */
export function dateDaysAgo(n: number): string {
  return new Date(BASE - n * DAY).toISOString().slice(0, 10)
}

const ALPHABET = 'abcdefghijklmnopqrstuvwxyz0123456789'
export function randomString(len: number): string {
  let out = ''
  for (let i = 0; i < len; i += 1) {
    out += ALPHABET[Math.floor(Math.random() * ALPHABET.length)]
  }
  return out
}

/** Deterministic-ish pseudo pick so generated rows vary across calls. */
export function pick<T>(values: readonly T[]): T {
  return values[Math.floor(Math.random() * values.length)]
}

/** A short pseudo message-id like `<abc123@mta1.example.net>`. */
export function messageId(host = 'mta1.example.net'): string {
  return `<${randomString(10)}.${Date.now().toString(36)}@${host}>`
}
