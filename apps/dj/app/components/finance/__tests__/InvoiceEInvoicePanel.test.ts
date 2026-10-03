import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import InvoiceEInvoicePanel from '../InvoiceEInvoicePanel.vue'
import { useInvoiceStore } from '../../../stores/invoice'
import { makeInvoice } from './fixtures'

function setup(answer: Awaited<ReturnType<ReturnType<typeof useInvoiceStore>['fetchEInvoiceCheck']>> | Error) {
  setActivePinia(createPinia())
  const store = useInvoiceStore()
  const spy = vi.spyOn(store, 'fetchEInvoiceCheck')
  if (answer instanceof Error) spy.mockRejectedValue(answer)
  else spy.mockResolvedValue(answer)
  const wrapper = mount(InvoiceEInvoicePanel, { props: { invoice: makeInvoice({ id: 'inv-9', status: 'issued' }) } })
  return { wrapper, spy }
}

const open = async (w: ReturnType<typeof mount>) => { await w.get('.iep-toggle').trigger('click'); await flushPromises() }

describe('InvoiceEInvoicePanel', () => {
  beforeEach(() => { document.body.innerHTML = '' })

  it('does nothing until opened', () => {
    const { wrapper, spy } = setup({ format: 'facturx', ready: true, problems: [] })
    expect(spy).not.toHaveBeenCalled()
    expect(wrapper.find('.iep-body').exists()).toBe(false)
  })

  it('offers the download only when the server says the invoice passes', async () => {
    const { wrapper, spy } = setup({ format: 'facturx', ready: true, problems: [] })
    await open(wrapper)
    expect(spy).toHaveBeenCalledWith('inv-9', 'facturx')
    expect(wrapper.get('a.iep-download').attributes('href')).toBe('/api/v1/finance/invoices/inv-9/einvoice?format=facturx')
  })

  it('does not claim more than it checked: Factur-X vs XRechnung wording', async () => {
    const { wrapper } = setup({ format: 'facturx', ready: true, problems: [] })
    await open(wrapper)
    expect(wrapper.text()).toContain("Passes KlubHub's EN 16931 checks.")
    expect(wrapper.text()).not.toContain('KoSIT')

    await wrapper.findAll('[role=radio]')[1]!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain("Passes KlubHub's EN 16931 and XRechnung checks.")
    expect(wrapper.text()).toContain('Not run through the official KoSIT validator')
    expect(wrapper.text()).not.toContain('every EN 16931 rule')

    await wrapper.findAll('[role=radio]')[2]!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('official KoSIT validator')
  })

  it('lists what is missing instead of a download', async () => {
    const { wrapper } = setup({ format: 'facturx', ready: false, problems: [{ field: 'buyer.city', message: 'Buyer city is required' }] })
    await open(wrapper)
    expect(wrapper.find('a.iep-download').exists()).toBe(false)
    expect(wrapper.text()).toContain('buyer.city')
    expect(wrapper.text()).toContain('Buyer city is required')
  })

  it('re-checks when another format is chosen', async () => {
    const { wrapper, spy } = setup({ format: 'facturx', ready: true, problems: [] })
    await open(wrapper)
    await wrapper.findAll('[role=radio]')[2]!.trigger('click')
    await flushPromises()
    expect(spy).toHaveBeenLastCalledWith('inv-9', 'xrechnung-ubl')
    expect(wrapper.get('a.iep-download').attributes('href')).toContain('format=xrechnung-ubl')
  })

  it('says so when the server has no generator', async () => {
    const { wrapper } = setup(null)
    await open(wrapper)
    expect(wrapper.text()).toContain("isn't enabled")
    expect(wrapper.find('a.iep-download').exists()).toBe(false)
  })

  it('shows a failure as an alert', async () => {
    const { wrapper } = setup(new Error('boom'))
    await open(wrapper)
    expect(wrapper.get('[role=alert]').text()).toBe('boom')
  })
})
