import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import VmtasPage from '@/pages/outbound/VmtasPage.vue'
import { outboundConfigService } from '@/services'
import type { Listener, VMTA } from '@/types'

const sampleVmta: VMTA = {
  id: 'vmta-1',
  name: 'vmta-east-1',
  status: 'active',
  notes: 'primary egress',
  listenerId: 'lst-1',
  listenerName: 'esmtp-east-1',
  ipAddress: '203.0.113.10',
  ehloName: 'mail.example.com',
  maxConnections: 0,
}

const sampleListener: Listener = {
  id: 'lst-1',
  name: 'esmtp-east-1',
  ipAddress: '203.0.113.10',
  port: 25,
  hostname: 'mail.example.com',
  tlsEnabled: false,
  tlsCertPath: '',
  tlsKeyPath: '',
  requireAuth: false,
  maxMessageSize: '0',
  relayHosts: [],
  status: 'active',
}

vi.mock('@/services', () => {
  return {
    outboundConfigService: {
      listVmtas: vi.fn(),
      updateVmta: vi.fn(),
      createVmta: vi.fn(),
      listListeners: vi.fn(),
    },
  }
})

// useToast pushes to a shared store; stub it to avoid DOM side effects.
vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ toast: vi.fn() }),
}))

describe('VmtasPage edit dialog', () => {
  beforeEach(() => {
    vi.mocked(outboundConfigService.listVmtas).mockResolvedValue({ items: [sampleVmta] })
    vi.mocked(outboundConfigService.listListeners).mockResolvedValue({ items: [sampleListener] })
    vi.mocked(outboundConfigService.updateVmta).mockResolvedValue(sampleVmta)
  })

  it('opens the edit dialog prefilled with a listener dropdown and calls updateVmta', async () => {
    const wrapper = mount(VmtasPage, { attachTo: document.body })
    await flushPromises()

    // Row rendered with the listener-based columns.
    expect(document.body.textContent).toContain('vmta-east-1')
    expect(document.body.textContent).toContain('esmtp-east-1')
    expect(document.body.textContent).toContain('203.0.113.10')

    // Open edit dialog for the row.
    const editBtn = document.body.querySelector(
      '[data-testid="edit-vmta-vmta-1"]',
    ) as HTMLButtonElement
    expect(editBtn).toBeTruthy()
    editBtn.click()
    await flushPromises()

    expect(document.body.textContent).toContain('Edit VMTA')

    // Form is prefilled from the row.
    const nameInput = document.body.querySelector('#vmta-name') as HTMLInputElement
    expect(nameInput.value).toBe('vmta-east-1')
    const notesInput = document.body.querySelector('#vmta-notes') as HTMLInputElement
    expect(notesInput.value).toBe('primary egress')
    const statusSelect = document.body.querySelector('#vmta-status') as HTMLSelectElement
    expect(statusSelect.value).toBe('active')

    // The Listener dropdown is present and prefilled from the row, and the
    // resolved IP/EHLO read-only fields reflect the selected listener.
    const listenerSelect = document.body.querySelector(
      '[data-testid="vmta-listener"]',
    ) as HTMLSelectElement
    expect(listenerSelect.value).toBe('lst-1')
    const ipInput = document.body.querySelector('#vmta-ip') as HTMLInputElement
    expect(ipInput.value).toBe('203.0.113.10')
    expect(ipInput.disabled).toBe(true)

    // Change a couple of fields and submit.
    notesInput.value = 'updated notes'
    notesInput.dispatchEvent(new Event('input'))
    statusSelect.value = 'disabled'
    statusSelect.dispatchEvent(new Event('change'))
    const maxConn = document.body.querySelector('#vmta-max-conn') as HTMLInputElement
    maxConn.value = '5'
    maxConn.dispatchEvent(new Event('input'))
    await flushPromises()

    const form = document.body.querySelector('form') as HTMLFormElement
    form.dispatchEvent(new Event('submit'))
    await flushPromises()

    expect(outboundConfigService.updateVmta).toHaveBeenCalledTimes(1)
    expect(outboundConfigService.updateVmta).toHaveBeenCalledWith('vmta-1', {
      name: 'vmta-east-1',
      listener_id: 'lst-1',
      max_connections: 5,
      status: 'disabled',
      notes: 'updated notes',
    })

    wrapper.unmount()
  })
})
