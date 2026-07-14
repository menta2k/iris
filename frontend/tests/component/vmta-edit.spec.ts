import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { VSelect } from 'vuetify/components'
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
  role: 'inbound',
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

// The id/data-testid may land on an inner element of the v-select, so locate
// the component whose rendered markup contains the marker attribute.
function findSelect(wrapper: VueWrapper, marker: string) {
  const sel = wrapper.findAllComponents(VSelect).find((c) => c.html().includes(marker))
  if (!sel) throw new Error(`v-select matching ${marker} not found`)
  return sel
}

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
    const statusSelect = findSelect(wrapper, 'id="vmta-status"')
    expect(statusSelect.props('modelValue')).toBe('active')

    // IP/EHLO are now EDITABLE fields owned by the VMTA, prefilled from the row;
    // the listener dropdown is an optional association, also prefilled.
    const listenerSelect = findSelect(wrapper, 'data-testid="vmta-listener"')
    expect(listenerSelect.props('modelValue')).toBe('lst-1')
    // The dropdown offers the loaded listeners (plus the "none" placeholder).
    expect(listenerSelect.props('items')).toEqual([
      { title: '— None —', value: '' },
      { title: 'esmtp-east-1 (203.0.113.10:25)', value: 'lst-1' },
    ])
    const ipInput = document.body.querySelector('#vmta-ip') as HTMLInputElement
    expect(ipInput.value).toBe('203.0.113.10')
    expect(ipInput.disabled).toBe(false)
    const ehloInput = document.body.querySelector('#vmta-ehlo') as HTMLInputElement
    expect(ehloInput.value).toBe('mail.example.com')
    expect(ehloInput.disabled).toBe(false)

    // Change a couple of fields and submit.
    ipInput.value = '203.0.113.99'
    ipInput.dispatchEvent(new Event('input'))
    notesInput.value = 'updated notes'
    notesInput.dispatchEvent(new Event('input'))
    await statusSelect.setValue('disabled')
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
      ip_address: '203.0.113.99',
      ehlo_name: 'mail.example.com',
      listener_id: 'lst-1',
      max_connections: 5,
      tls_mode: '',
      status: 'disabled',
      notes: 'updated notes',
      node_id: '',
    })

    wrapper.unmount()
  })
})
