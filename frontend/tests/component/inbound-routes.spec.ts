import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import InboundRoutesPage from '@/pages/inbound/InboundRoutesPage.vue'
import { inboundAutomationService } from '@/services'
import type { InboundRoute } from '@/types'

const maildirRoute: InboundRoute = {
  id: 'r-md',
  name: 'archive',
  matchType: 'recipient_domain',
  matchValue: 'archive.example.com',
  action: 'maildir',
  priority: 0,
  status: 'active',
  forwardHost: '',
  forwardPort: 25,
  forwardTls: 'opportunistic',
  maildirPath: '',
  destinationUrl: '',
  timeoutSeconds: 10,
}
const forwardRoute: InboundRoute = {
  ...maildirRoute,
  id: 'r-fw',
  name: 'relay',
  matchValue: 'legacy.example.com',
  action: 'forward',
  forwardHost: 'mail.internal',
  forwardPort: 2525,
  forwardTls: 'required',
}

vi.mock('@/services', () => ({
  inboundAutomationService: {
    listInboundRoutes: vi.fn(),
    createInboundRoute: vi.fn(),
    updateInboundRoute: vi.fn(),
    deleteInboundRoute: vi.fn(),
  },
}))

vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ toast: vi.fn() }),
}))

describe('InboundRoutesPage', () => {
  beforeEach(() => {
    vi.mocked(inboundAutomationService.listInboundRoutes).mockResolvedValue({
      items: [maildirRoute, forwardRoute],
    })
    vi.mocked(inboundAutomationService.createInboundRoute).mockResolvedValue(maildirRoute)
  })

  it('summarizes each action target in the table', async () => {
    const wrapper = mount(InboundRoutesPage, { attachTo: document.body })
    await flushPromises()

    const text = document.body.textContent ?? ''
    expect(text).toContain('(default base)') // maildir with no explicit path
    expect(text).toContain('mail.internal:2525 (required)') // forward smarthost
    expect(text).toContain('archive.example.com')

    wrapper.unmount()
  })

  it('reveals forward fields and submits a forward payload', async () => {
    const wrapper = mount(InboundRoutesPage, { attachTo: document.body })
    await flushPromises()

    ;(document.body.querySelector('[data-testid="create-inbound-route"]') as HTMLButtonElement).click()
    await flushPromises()

    // Switch the action to forward; the smarthost field must appear.
    const actionSelect = document.body.querySelector('#ir-action') as HTMLSelectElement
    actionSelect.value = 'forward'
    actionSelect.dispatchEvent(new Event('change'))
    await flushPromises()

    expect(document.body.querySelector('#ir-fwd-host')).not.toBeNull()
    expect(document.body.querySelector('#ir-maildir')).toBeNull()

    const set = (sel: string, val: string) => {
      const el = document.body.querySelector(sel) as HTMLInputElement
      el.value = val
      el.dispatchEvent(new Event('input'))
    }
    set('#ir-name', 'relay-legacy')
    set('#ir-match-value', 'legacy.example.com')
    set('#ir-fwd-host', 'mail.internal')
    await flushPromises()

    const submit = Array.from(document.body.querySelectorAll('form button')).find(
      (b) => b.textContent?.trim() === 'Create',
    ) as HTMLButtonElement
    submit.click()
    await flushPromises()

    expect(inboundAutomationService.createInboundRoute).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'relay-legacy', action: 'forward', forward_host: 'mail.internal' }),
    )

    wrapper.unmount()
  })
})
