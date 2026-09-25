import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import InvoiceList from '../InvoiceList.vue'
import { useInvoiceStore, FinanceApiError } from '../../../stores/invoice'
import { makeInvoice } from './fixtures'

function setup(prepare: (store: ReturnType<typeof useInvoiceStore>) => void = () => undefined) {
  const store = useInvoiceStore()
  prepare(store)
  const wrapper = mount(InvoiceList, { attachTo: document.body })
  return { store, wrapper }
}

describe('InvoiceList', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    document.body.innerHTML = ''
  })

  function seed(store: ReturnType<typeof useInvoiceStore>) {
    store.invoices = [
      makeInvoice({ id: 'draft', status: 'draft', invoice_number: null, issued_at: null, outstanding_minor: 0 }),
      makeInvoice({ id: 'issued', status: 'issued' }),
      makeInvoice({ id: 'overdue', status: 'issued', invoice_number: 'INV-0002-EUR', due_at: '2000-01-01T00:00:00Z' }),
      makeInvoice({ id: 'paid', status: 'paid', invoice_number: 'INV-0003-EUR', outstanding_minor: 0, received_minor: 119000 }),
      makeInvoice({ id: 'cn', kind: 'credit_note', status: 'issued', invoice_number: 'CN-0001-EUR', outstanding_minor: 0 }),
      makeInvoice({ id: 'credited', status: 'credited', invoice_number: 'INV-0004-EUR', outstanding_minor: 0 }),
    ]
    store.listLoaded = true
  }

  it('renders rows as buttons with a full aria-label, credit notes negative', () => {
    const { wrapper } = setup(seed)
    const rows = wrapper.findAll('button.il-row')
    expect(rows).toHaveLength(6)
    const draftLabel = rows[0]!.attributes('aria-label')!
    expect(draftLabel).toMatch(/^Draft, Club Alpha, 20 SEPT? 2026, total €1,190.00, DRAFT$/)
    expect(rows[1]!.attributes('aria-label')).toContain('outstanding €1,190.00')
    expect(rows[4]!.text()).toContain('-€1,190.00')
    expect(rows[4]!.attributes('aria-label')).toContain('credit note')
  })

  it('status chips form a radiogroup and filter overdue/void', async () => {
    const { wrapper, store } = setup(seed)
    const group = wrapper.get('[role="radiogroup"]')
    const chips = group.findAll('[role="radio"]')
    expect(chips.map((c) => c.text().replace(/\d+$/, ''))).toEqual(['ALL', 'DRAFT', 'ISSUED', 'OVERDUE', 'PAID', 'VOID'])
    expect(chips[0]!.attributes('aria-checked')).toBe('true')

    await chips[3]!.trigger('click')
    expect(store.filter).toBe('overdue')
    expect(wrapper.findAll('button.il-row').map((r) => r.attributes('aria-label'))).toEqual([
      expect.stringContaining('INV-0002-EUR'),
    ])

    await chips[5]!.trigger('click')
    // void = cancelled | credited | corrected; the issued credit note is not void
    expect(wrapper.findAll('button.il-row')).toHaveLength(1)
    expect(wrapper.text()).toContain('CREDITED')
  })

  it('arrow keys move the checked chip (roving tabindex)', async () => {
    const { wrapper, store } = setup(seed)
    const chips = wrapper.findAll('[role="radio"]')
    expect(chips[0]!.attributes('tabindex')).toBe('0')
    expect(chips[1]!.attributes('tabindex')).toBe('-1')
    await chips[0]!.trigger('keydown', { key: 'ArrowRight' })
    expect(store.filter).toBe('draft')
    await chips[1]!.trigger('keydown', { key: 'End' })
    expect(store.filter).toBe('void')
  })

  it('shows the filter-empty state with SHOW ALL', async () => {
    const { wrapper, store } = setup((s) => {
      s.invoices = [makeInvoice({ status: 'draft', invoice_number: null })]
      s.listLoaded = true
      s.setFilter('paid')
    })
    expect(wrapper.text()).toContain('No PAID invoices.')
    await wrapper.findAll('button').find((b) => b.text() === 'SHOW ALL')!.trigger('click')
    expect(store.filter).toBe('all')
  })

  it('shows the empty state with a create button', async () => {
    const { wrapper, store } = setup((s) => { s.listLoaded = true })
    const spy = vi.spyOn(store, 'openCreate')
    expect(wrapper.text()).toContain('No invoices yet. Bill a played gig in two clicks.')
    await wrapper.findAll('button').find((b) => b.text().includes('CREATE INVOICE'))!.trigger('click')
    expect(spy).toHaveBeenCalled()
  })

  it('shows a skeleton while the first load runs', () => {
    const { wrapper } = setup((s) => { s.listLoading = true })
    expect(wrapper.find('[aria-busy="true"]').exists()).toBe(true)
  })

  it('never flashes the empty state before the first load', () => {
    const { wrapper } = setup()
    expect(wrapper.text()).not.toContain('No invoices yet')
    expect(wrapper.find('[aria-busy="true"]').exists()).toBe(true)
  })

  it('shows the error with RETRY', async () => {
    const { wrapper, store } = setup((s) => {
      s.listError = new FinanceApiError(500, 'unknown', 'Request failed.')
    })
    const spy = vi.spyOn(store, 'fetchInvoices').mockResolvedValue()
    expect(wrapper.get('[role="alert"]').text()).toContain('Request failed.')
    await wrapper.findAll('button').find((b) => b.text() === 'RETRY')!.trigger('click')
    expect(spy).toHaveBeenCalled()
  })

  it('shows the 503 message when invoicing is off', () => {
    const { wrapper } = setup((s) => { s.disabled = true })
    expect(wrapper.text()).toContain("Invoicing isn't enabled on this server.")
    expect(wrapper.findAll('[role="radio"]').every((c) => c.attributes('disabled') !== undefined)).toBe(true)
  })

  it('opens the detail sheet from a row', async () => {
    const { wrapper, store } = setup(seed)
    const spy = vi.spyOn(store, 'openDetail').mockResolvedValue(null)
    await wrapper.findAll('button.il-row')[1]!.trigger('click')
    expect(spy).toHaveBeenCalledWith('issued')
  })
})
