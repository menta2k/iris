import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import RetentionPage from '@/pages/operations/RetentionPage.vue'
import { retentionService } from '@/services'
import type { RetentionView } from '@/types'

const mailRecords: RetentionView = {
  policy: { tableName: 'mail_records', retentionDays: 90, compressAfterDays: 7, enabled: true },
  label: 'Mail logs',
  hypertable: true,
  chunkCount: 12,
  compressedChunks: 8,
  totalBytes: 5 * 1024 * 1024 * 1024,
  compressedBytes: 1 * 1024 * 1024 * 1024,
  uncompressedBytes: 4 * 1024 * 1024 * 1024,
  oldestData: '2026-04-01T00:00:00Z',
  lastRun: {
    id: 'r1',
    tableName: 'mail_records',
    startedAt: '2026-06-28T00:00:00Z',
    chunksCompressed: 2,
    chunksDropped: 1,
    bytesBefore: 6 * 1024 * 1024 * 1024,
    bytesAfter: 5 * 1024 * 1024 * 1024,
  },
}
const plainTable: RetentionView = {
  policy: { tableName: 'audit_entries', retentionDays: 0, compressAfterDays: 0, enabled: false },
  label: 'Audit log',
  hypertable: false,
  chunkCount: 0,
  compressedChunks: 0,
  totalBytes: 0,
  compressedBytes: 0,
  uncompressedBytes: 0,
}

vi.mock('@/services', () => ({
  retentionService: {
    listRetention: vi.fn(),
    updateRetention: vi.fn(),
    runRetention: vi.fn(),
  },
}))
vi.mock('@/composables/useToast', () => ({ useToast: () => ({ toast: vi.fn() }) }))

describe('RetentionPage', () => {
  beforeEach(() => {
    vi.mocked(retentionService.listRetention).mockResolvedValue({ items: [mailRecords, plainTable] })
    vi.mocked(retentionService.updateRetention).mockResolvedValue(mailRecords.policy)
  })

  it('shows per-table disk stats and a freed-space summary, and flags non-hypertables', async () => {
    const wrapper = mount(RetentionPage, { attachTo: document.body })
    await flushPromises()

    const text = document.body.textContent ?? ''
    expect(text).toContain('5.0 GB') // total on disk
    expect(text).toContain('1.0 GB compressed') // compressed footprint
    expect(text).toContain('−1.0 GB') // freed by last run (6 -> 5 GB)
    expect(text).toContain('90d') // keep window
    // Non-hypertable row is clearly marked unavailable.
    expect(text).toContain('TimescaleDB not enabled')

    wrapper.unmount()
  })

  it('rejects a compress-after >= keep window before saving', async () => {
    const wrapper = mount(RetentionPage, { attachTo: document.body })
    await flushPromises()

    ;(document.body.querySelector('[data-testid="edit-retention-mail_records"]') as HTMLButtonElement).click()
    await flushPromises()

    const set = (sel: string, val: string) => {
      const el = document.body.querySelector(sel) as HTMLInputElement
      el.value = val
      el.dispatchEvent(new Event('input'))
    }
    set('#ret-keep', '10')
    set('#ret-compress', '10') // not allowed: must be < keep
    await flushPromises()

    const submit = Array.from(document.body.querySelectorAll('form button')).find(
      (b) => b.textContent?.trim() === 'Save',
    ) as HTMLButtonElement
    submit.click()
    await flushPromises()

    expect(retentionService.updateRetention).not.toHaveBeenCalled()

    wrapper.unmount()
  })
})
