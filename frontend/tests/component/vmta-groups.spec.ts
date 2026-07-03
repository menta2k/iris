import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { VSelect } from 'vuetify/components'
import VmtaGroupsPage from '@/pages/outbound/VmtaGroupsPage.vue'
import { outboundConfigService } from '@/services'
import type { VMTA, VMTAGroup } from '@/types'

const vmtaA: VMTA = {
  id: 'vmta-a',
  name: 'east-1',
  status: 'active',
  notes: '',
  listenerId: '',
  listenerName: '',
  ipAddress: '203.0.113.10',
  ehloName: 'a.example.com',
  maxConnections: 0,
}
const vmtaB: VMTA = { ...vmtaA, id: 'vmta-b', name: 'east-2', ipAddress: '203.0.113.11' }

const sampleGroup: VMTAGroup = {
  id: 'grp-1',
  name: 'bulk-pool',
  status: 'active',
  members: [
    { vmtaId: 'vmta-a', weight: 3 },
    { vmtaId: 'vmta-b', weight: 1 },
  ],
}

vi.mock('@/services', () => ({
  outboundConfigService: {
    listVmtaGroups: vi.fn(),
    listVmtas: vi.fn(),
    createVmtaGroup: vi.fn(),
    updateVmtaGroup: vi.fn(),
  },
}))

vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ toast: vi.fn() }),
}))

describe('VmtaGroupsPage', () => {
  beforeEach(() => {
    vi.mocked(outboundConfigService.listVmtaGroups).mockResolvedValue({ items: [sampleGroup] })
    vi.mocked(outboundConfigService.listVmtas).mockResolvedValue({ items: [vmtaA, vmtaB] })
    vi.mocked(outboundConfigService.createVmtaGroup).mockResolvedValue(sampleGroup)
  })

  it('resolves member ids to names and shows effective weight percentages', async () => {
    const wrapper = mount(VmtaGroupsPage, { attachTo: document.body })
    await flushPromises()

    const text = document.body.textContent ?? ''
    // Member badges show the VMTA name (not the raw id) and the % of the pool
    // (3 and 1 => 75% and 25%), never the opaque id.
    expect(text).toContain('east-1 (203.0.113.10) · 75%')
    expect(text).toContain('east-2 (203.0.113.11) · 25%')
    expect(text).not.toContain('vmta-a · w3')

    wrapper.unmount()
  })

  it('prevents picking the same VMTA in two member rows', async () => {
    const wrapper = mount(VmtaGroupsPage, { attachTo: document.body })
    await flushPromises()

    ;(document.body.querySelector('[data-testid="create-vmta-group"]') as HTMLButtonElement).click()
    await flushPromises()

    // Add two member rows; addMember auto-picks distinct VMTAs.
    const addBtns = Array.from(document.body.querySelectorAll('button')).filter(
      (b) => b.textContent?.trim() === 'Add member',
    ) as HTMLButtonElement[]
    addBtns[0].click()
    await flushPromises()
    addBtns[0].click()
    await flushPromises()

    // In create mode the only v-selects in the dialog are the member dropdowns.
    const selects = wrapper.findAllComponents(VSelect)
    // The two member dropdowns must hold different VMTAs (no duplicate).
    expect(selects.length).toBeGreaterThanOrEqual(2)
    const firstChoice = selects[0].props('modelValue')
    expect(firstChoice).not.toBe(selects[1].props('modelValue'))
    // In the second dropdown, the item already chosen by the first is disabled.
    const secondItems = selects[1].props('items') as Array<{
      value: string
      props?: { disabled?: boolean }
    }>
    const dupItem = secondItems.find((i) => i.value === firstChoice)
    expect(dupItem?.props?.disabled).toBe(true)

    wrapper.unmount()
  })
})
