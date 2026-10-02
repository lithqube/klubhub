import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import InvoiceCreateDialog from '../InvoiceCreateDialog.vue'
import { useInvoiceStore } from '../../../stores/invoice'
import { makeInvoice } from './fixtures'

const wrappers: VueWrapper[] = []
const fetchMock = vi.fn()
const slotStub = { template: '<div><slot /></div>' }
function render() {
  const wrapper = mount(InvoiceCreateDialog, {
    props: { open: true, presetGigId: 'gig-1' },
    // Only the portal/focus shell is stubbed; form, picker and Pinia actions are real.
    global: { stubs: { Dialog: slotStub, DialogContent: slotStub, DialogTitle: slotStub, DialogDescription: slotStub } },
  })
  wrappers.push(wrapper)
  return wrapper
}
beforeEach(() => {
  setActivePinia(createPinia())
  fetchMock.mockReset()
  vi.stubGlobal('$fetch', fetchMock)
  fetchMock.mockImplementation(async (url: string) => {
    if (url === '/api/v1/gigs') return { data: [{ id: 'gig-1', date: '2026-10-01', venue: 'Actual venue', status: 'played', fee_amount: 500, fee_currency: 'EUR' }] }
    if (url === '/api/v1/finance/invoices') return { data: [] }
    throw new Error(`Unexpected request: ${url}`)
  })
})
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.unstubAllGlobals() })

describe('invoice create form with real transport-backed Pinia', () => {
  it('only announces creation after the draft POST succeeds, never implying sending', async () => {
    const wrapper = render()
    await flushPromises()
    const draft = makeInvoice({ gig_id: 'gig-1', status: 'draft' })
    let resolve!: (value: unknown) => void
    const pending = new Promise(r => { resolve = r })
    fetchMock.mockImplementation((url: string, opts?: { method?: string }) => {
      if (url === '/api/v1/finance/invoices' && opts?.method === 'POST') return pending
      throw new Error(`Unexpected request: ${url}`)
    })
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('created')).toBeUndefined()
    expect(useInvoiceStore().invoices).toEqual([])
    expect(wrapper.text()).toContain('Nothing is sent to the promoter')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/finance/invoices', {
      method: 'POST', body: { gig_id: 'gig-1', due_at: '2026-10-15T00:00:00Z', number_prefix: 'INV' },
    })
    resolve({ data: draft })
    await flushPromises()
    expect(wrapper.emitted('created')).toEqual([[draft]])
    expect(wrapper.emitted('update:open')).toEqual([[false]])
    expect(useInvoiceStore().invoices).toEqual([draft])
  })

  it('shows rejected creation without fake success, closing or a fixture fallback', async () => {
    const wrapper = render()
    await flushPromises()
    fetchMock.mockRejectedValueOnce({ statusCode: 400, data: { error: 'validation_failed', message: 'Billing profile is incomplete' } })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Billing profile is incomplete')
    expect(wrapper.emitted('created')).toBeUndefined()
    expect(wrapper.emitted('update:open')).toBeUndefined()
    expect(useInvoiceStore().invoices).toEqual([])
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })
})
