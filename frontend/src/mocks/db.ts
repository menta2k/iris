// In-memory mock database. Seeded once from fixtures at module load; mutations
// (create/update/remove) replace arrays immutably so handlers stay pure-ish and
// the UI reflects changes live within a dev session. Reset by restarting vite.

import type {
  AcmeCertificate,
  AuditEntry,
  AutomationRule,
  BounceRule,
  Bounce,
  DeliveryBlueprint,
  DkimDomain,
  DmarcReport,
  FeedbackLoop,
  FeedbackReport,
  InboundRoute,
  Listener,
  MailRecord,
  Queue,
  RoutingRule,
  RspamdResult,
  SubjectClassification,
  Suppression,
  TLSPolicy,
  User,
  VMTA,
  VMTAGroup,
  WarmupSchedule,
  WorkerErrorLog,
} from '../types'
import type { Query } from './router'
import { seedData } from './fixtures'

export interface MockData {
  users: User[]
  auditEntries: AuditEntry[]
  listeners: Listener[]
  vmtas: VMTA[]
  vmtaGroups: VMTAGroup[]
  routingRules: RoutingRule[]
  warmupSchedules: WarmupSchedule[]
  blueprints: DeliveryBlueprint[]
  automationRules: AutomationRule[]
  bounceRules: BounceRule[]
  mailRecords: MailRecord[]
  bounces: Bounce[]
  feedbackReports: FeedbackReport[]
  queues: Queue[]
  workerErrors: WorkerErrorLog[]
  dkimDomains: DkimDomain[]
  suppressions: Suppression[]
  tlsPolicies: TLSPolicy[]
  inboundRoutes: InboundRoute[]
  rspamdResults: RspamdResult[]
  feedbackLoops: FeedbackLoop[]
  classifications: SubjectClassification[]
  dmarcReports: DmarcReport[]
  acmeCertificates: AcmeCertificate[]
}

function clone(data: MockData): MockData {
  const copy = {} as MockData
  ;(Object.keys(data) as Array<keyof MockData>).forEach((key) => {
    copy[key] = data[key].slice() as never
  })
  return copy
}

let store: MockData = clone(seedData)

export function all<K extends keyof MockData>(name: K): MockData[K] {
  return store[name]
}

export function createRow<K extends keyof MockData>(
  name: K,
  row: MockData[K][number],
): MockData[K][number] {
  store = { ...store, [name]: [...store[name], row] } as unknown as MockData
  return row
}

export function updateRow<K extends keyof MockData>(
  name: K,
  id: string,
  patch: Partial<MockData[K][number]>,
): MockData[K][number] | undefined {
  const rows = store[name] as unknown as Array<{ id: string }>
  let updated: MockData[K][number] | undefined
  const next = rows.map((row) => {
    if (row.id !== id) return row
    updated = { ...row, ...patch } as MockData[K][number]
    return updated
  }) as unknown as MockData[K]
  store = { ...store, [name]: next } as unknown as MockData
  return updated
}

export function removeRow<K extends keyof MockData>(name: K, id: string): boolean {
  const rows = store[name] as unknown as Array<{ id: string }>
  const next = rows.filter((row) => row.id !== id)
  const changed = next.length !== rows.length
  if (changed) {
    store = { ...store, [name]: next as unknown as MockData[K] } as unknown as MockData
  }
  return changed
}

export function findRow<K extends keyof MockData>(
  name: K,
  id: string,
): MockData[K][number] | undefined {
  const rows = store[name] as unknown as Array<{ id: string }>
  return rows.find((row) => row.id === id) as MockData[K][number] | undefined
}

// -- id generation ----------------------------------------------------------

let counter = 0
export function genId(prefix: string): string {
  counter += 1
  return `${prefix}_${Date.now().toString(36)}${counter.toString(36)}`
}

// -- cursor pagination ------------------------------------------------------

export interface PagedResult<T> {
  items: T[]
  page: { nextPageToken?: string }
}

/** Slice `rows` by the API's opaque offset token (`page.page_size` /
 *  `page.page_token` query params). An optional `filter` is applied before
 *  paging so filtered lists also paginate correctly. */
export function paged<T>(
  rows: T[],
  query: Query,
  options: { defaultSize?: number; filter?: (row: T) => boolean } = {},
): PagedResult<T> {
  const filtered = options.filter ? rows.filter(options.filter) : rows
  const size = clamp(
    parseInt(query['page.page_size'] ?? '', 10) || options.defaultSize || 50,
    1,
    500,
  )
  const offset = decodeToken(query['page.page_token'])
  const items = filtered.slice(offset, offset + size)
  const nextOffset = offset + size
  const nextPageToken =
    nextOffset < filtered.length ? encodeToken(nextOffset) : undefined
  return nextPageToken
    ? { items, page: { nextPageToken } }
    : { items, page: {} }
}

function encodeToken(offset: number): string {
  return `mock:${offset}`
}

function decodeToken(token: string | undefined): number {
  if (!token) return 0
  const match = /^mock:(\d+)$/.exec(token)
  return match ? parseInt(match[1], 10) : 0
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value))
}
