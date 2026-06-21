import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ListenersPage from '@/pages/outbound/ListenersPage.vue'
import { outboundConfigService } from '@/services'
import type { Listener } from '@/types'

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

vi.mock('@/services', () => ({
  outboundConfigService: {
    listListeners: vi.fn(),
    createListener: vi.fn(),
    updateListener: vi.fn(),
  },
}))

vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ toast: vi.fn() }),
}))

describe('ListenersPage create dialog', () => {
  beforeEach(() => {
    vi.mocked(outboundConfigService.listListeners).mockResolvedValue({ items: [sampleListener] })
    vi.mocked(outboundConfigService.createListener).mockResolvedValue(sampleListener)
  })

  it('renders the list and opens the create dialog with sensible defaults', async () => {
    const wrapper = mount(ListenersPage, { attachTo: document.body })
    await flushPromises()

    // Row rendered with ip:port.
    expect(document.body.textContent).toContain('esmtp-east-1')
    expect(document.body.textContent).toContain('203.0.113.10:25')

    const createBtn = document.body.querySelector(
      '[data-testid="create-listener"]',
    ) as HTMLButtonElement
    createBtn.click()
    await flushPromises()

    expect(document.body.textContent).toContain('Create Listener')
    const portInput = document.body.querySelector('#listener-port') as HTMLInputElement
    expect(portInput.value).toBe('25')

    wrapper.unmount()
  })

  it('reveals cert/key inputs only when TLS is toggled on', async () => {
    const wrapper = mount(ListenersPage, { attachTo: document.body })
    await flushPromises()

    const createBtn = document.body.querySelector(
      '[data-testid="create-listener"]',
    ) as HTMLButtonElement
    createBtn.click()
    await flushPromises()

    // Cert/key inputs hidden by default (TLS off).
    expect(document.body.querySelector('#listener-cert')).toBeFalsy()
    expect(document.body.querySelector('#listener-key')).toBeFalsy()

    // Toggle TLS on.
    const tlsToggle = document.body.querySelector(
      '[data-testid="listener-tls"]',
    ) as HTMLInputElement
    tlsToggle.checked = true
    tlsToggle.dispatchEvent(new Event('change'))
    await flushPromises()

    expect(document.body.querySelector('#listener-cert')).toBeTruthy()
    expect(document.body.querySelector('#listener-key')).toBeTruthy()

    wrapper.unmount()
  })
})
