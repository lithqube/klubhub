import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import InvoiceCreateForm from '../InvoiceCreateForm.vue'
import { useInvoiceStore } from '../../../stores/invoice'
import { makeInvoice } from './fixtures'

const wrappers: VueWrapper[] = []
const fetchMock = vi.fn()
// Nothing is stubbed: the form, the picker and the Pinia actions are real.
function render(props: { presetGigId?: string | null; focusSeq?: number } = { presetGigId: 'gig-1' }) {
  const wrapper = mount(InvoiceCreateForm, { props, attachTo: document.body })
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

describe('inline invoice create form with real transport-backed Pinia', () => {
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
    expect(wrapper.emitted('close')).toHaveLength(1)
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
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(useInvoiceStore().invoices).toEqual([])
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })

  it('is an inline section, not a dialog', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.get('section').attributes('aria-labelledby')).toBe('icf-title')
    expect(wrapper.get('#icf-title').text()).toBe('NEW INVOICE')
  })

  it('closes on CANCEL and on Escape without creating anything', async () => {
    const wrapper = render()
    await flushPromises()
    fetchMock.mockClear()
    await wrapper.findAll('button').find(b => b.text() === 'CANCEL')!.trigger('click')
    await wrapper.get('section').trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('close')).toHaveLength(2)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('moves focus to the gig field when it opens and when asked again', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = render({ presetGigId: null, focusSeq: 1 })
      await vi.advanceTimersByTimeAsync(100)
      expect(document.activeElement?.id).toBe('inv-create-gig')
      ;(document.activeElement as HTMLElement).blur()
      await wrapper.setProps({ focusSeq: 2 })
      await vi.advanceTimersByTimeAsync(100)
      expect(document.activeElement?.id).toBe('inv-create-gig')
    } finally {
      vi.useRealTimers()
    }
  })

  it('re-aims at another gig when asked to bill a different one while open', async () => {
    fetchMock.mockImplementation(async (url: string) => {
      if (url === '/api/v1/gigs') return { data: [
        { id: 'gig-1', date: '2026-10-01', venue: 'First', status: 'played', fee_amount: 500, fee_currency: 'EUR' },
        { id: 'gig-2', date: '2026-11-01', venue: 'Second', status: 'played', fee_amount: 700, fee_currency: 'EUR' },
      ] }
      if (url === '/api/v1/finance/invoices') return { data: [] }
      throw new Error(`Unexpected request: ${url}`)
    })
    const wrapper = render({ presetGigId: 'gig-1' })
    await flushPromises()
    expect((wrapper.get('#inv-create-gig').element as HTMLInputElement).value).toContain('First')
    await wrapper.setProps({ presetGigId: 'gig-2' })
    await flushPromises()
    expect((wrapper.get('#inv-create-gig').element as HTMLInputElement).value).toContain('Second')
  })
})
