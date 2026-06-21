import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

// Mock the services barrel used by the page.
const generate = vi.fn()
const apply = vi.fn()
vi.mock('@/services', () => ({
  kumoConfigService: {
    generate: (...args: unknown[]) => generate(...args),
    apply: (...args: unknown[]) => apply(...args),
  },
}))

// useToast is a singleton composable; stub it so toasts don't error.
vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ toast: vi.fn() }),
}))

import KumoConfig from '@/pages/operations/KumoConfig.vue'

const preview = {
  content: '-- generated kumomta policy\nkumo.on("init", function() end)',
  vmtaCount: 3,
  poolCount: 2,
  routeCount: 4,
  dkimCount: 1,
  suppressionCount: 5,
  checksum: 'abcdef1234567890',
}

describe('KumoConfig page', () => {
  beforeEach(() => {
    generate.mockReset()
    apply.mockReset()
  })

  it('renders the generated content and summary counts after generating', async () => {
    generate.mockResolvedValue(preview)
    const wrapper = mount(KumoConfig, { attachTo: document.body })

    // Before generating, a prompt placeholder is shown.
    expect(wrapper.text()).toContain('Generate / Preview')
    expect(wrapper.find('[data-testid="config-content"]').exists()).toBe(false)

    await wrapper.find('[data-testid="generate-config"]').trigger('click')
    await new Promise((r) => setTimeout(r, 0))
    await wrapper.vm.$nextTick()

    expect(generate).toHaveBeenCalledOnce()
    const content = wrapper.find('[data-testid="config-content"]')
    expect(content.exists()).toBe(true)
    expect(content.text()).toContain('generated kumomta policy')
    // Summary counts + short checksum.
    expect(wrapper.text()).toContain('abcdef123456')
    expect(wrapper.text()).toContain('3')
    wrapper.unmount()
  })

  it('requires type-to-confirm before applying the config', async () => {
    apply.mockResolvedValue({
      requestId: 'r1',
      status: 'APPLIED',
      checksum: 'abc',
      appliedPath: '/etc/kumomta/policy.lua',
      resultSummary: 'reloaded',
    })
    const wrapper = mount(KumoConfig, { attachTo: document.body })

    await wrapper.find('[data-testid="apply-config"]').trigger('click')
    await wrapper.vm.$nextTick()

    // Confirm dialog is open; the dialog footer "Apply" button (exact text) is
    // disabled until the confirmation token is typed.
    const confirmBtn = () =>
      Array.from(document.body.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Apply',
      ) as HTMLButtonElement
    expect(confirmBtn()).toBeTruthy()
    expect(confirmBtn().disabled).toBe(true)
    expect(apply).not.toHaveBeenCalled()

    const input = document.body.querySelector('#confirm-input') as HTMLInputElement
    input.value = 'APPLY'
    input.dispatchEvent(new Event('input'))
    await wrapper.vm.$nextTick()

    expect(confirmBtn().disabled).toBe(false)
    confirmBtn().click()
    await new Promise((r) => setTimeout(r, 0))
    await wrapper.vm.$nextTick()

    expect(apply).toHaveBeenCalledOnce()
    expect(apply.mock.calls[0][0]).toHaveProperty('confirmation_id')
    wrapper.unmount()
  })
})
