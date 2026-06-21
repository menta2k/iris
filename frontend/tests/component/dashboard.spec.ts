import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ServiceStatusWidget from '@/components/dashboard/ServiceStatusWidget.vue'
import QueueHealthWidget from '@/components/dashboard/QueueHealthWidget.vue'
import RecentMailActivity from '@/components/dashboard/RecentMailActivity.vue'
import ConfirmDialog from '@/components/confirm/ConfirmDialog.vue'
import type { MailRecord } from '@/types'

describe('ServiceStatusWidget', () => {
  it('renders the service state and a status badge', () => {
    const wrapper = mount(ServiceStatusWidget, { props: { state: 'RUNNING' } })
    expect(wrapper.text()).toContain('RUNNING')
    expect(wrapper.text()).toContain('KumoMTA Service')
  })

  it('falls back to "Unknown" when state is missing', () => {
    const wrapper = mount(ServiceStatusWidget, {})
    expect(wrapper.text()).toContain('Unknown')
  })
})

describe('QueueHealthWidget', () => {
  it('renders a formatted queued count', () => {
    const wrapper = mount(QueueHealthWidget, { props: { queued: 12345 } })
    expect(wrapper.text()).toContain('12,345')
  })

  it('shows zero when no value is provided', () => {
    const wrapper = mount(QueueHealthWidget, {})
    expect(wrapper.text()).toContain('0')
  })
})

describe('RecentMailActivity', () => {
  it('shows an empty state when there are no events', () => {
    const wrapper = mount(RecentMailActivity, { props: { events: [] } })
    expect(wrapper.text()).toContain('No recent mail events')
  })

  it('renders a row per mail event', () => {
    const events: MailRecord[] = [
      {
        id: '1',
        messageId: 'm-1',
        eventTime: '2026-06-20T10:00:00Z',
        mailclass: 'marketing',
        sender: 'a@example.com',
        recipient: 'b@gmail.com',
        recipientDomain: 'gmail.com',
        vmtaId: 'vmta-1',
        status: 'DELIVERED',
      },
    ]
    const wrapper = mount(RecentMailActivity, { props: { events } })
    expect(wrapper.text()).toContain('b@gmail.com')
    expect(wrapper.text()).toContain('marketing')
  })
})

describe('ConfirmDialog', () => {
  it('does not render when closed', () => {
    const wrapper = mount(ConfirmDialog, {
      props: { open: false, title: 'Danger' },
      attachTo: document.body,
    })
    expect(document.body.textContent).not.toContain('Danger')
    wrapper.unmount()
  })

  it('renders title and description when open', () => {
    const wrapper = mount(ConfirmDialog, {
      props: { open: true, title: 'Drain queue', description: 'This affects delivery.' },
      attachTo: document.body,
    })
    expect(document.body.textContent).toContain('Drain queue')
    expect(document.body.textContent).toContain('This affects delivery.')
    wrapper.unmount()
  })

  it('emits confirm only after typing the required confirmation text', async () => {
    const wrapper = mount(ConfirmDialog, {
      props: {
        open: true,
        title: 'Drain queue',
        confirmText: 'marketing',
        confirmLabel: 'Drain',
      },
      attachTo: document.body,
    })

    const confirmButton = () =>
      Array.from(document.body.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Drain',
      ) as HTMLButtonElement

    // Disabled until the user types the exact confirmation token.
    expect(confirmButton().disabled).toBe(true)

    const input = document.body.querySelector('#confirm-input') as HTMLInputElement
    input.value = 'marketing'
    input.dispatchEvent(new Event('input'))
    await wrapper.vm.$nextTick()

    expect(confirmButton().disabled).toBe(false)
    confirmButton().click()
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('confirm')).toBeTruthy()
    wrapper.unmount()
  })

  it('emits cancel when the cancel button is clicked', async () => {
    const onUpdate = vi.fn()
    const wrapper = mount(ConfirmDialog, {
      props: {
        open: true,
        title: 'Restart service',
        'onUpdate:open': onUpdate,
      },
      attachTo: document.body,
    })

    const cancel = Array.from(document.body.querySelectorAll('button')).find(
      (b) => b.textContent?.trim() === 'Cancel',
    ) as HTMLButtonElement
    cancel.click()
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('cancel')).toBeTruthy()
    expect(onUpdate).toHaveBeenCalledWith(false)
    wrapper.unmount()
  })
})
