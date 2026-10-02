import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import InvoiceReferences from '../InvoiceReferences.vue'
import InvoiceTotals from '../InvoiceTotals.vue'
import InvoicePdfLink from '../InvoicePdfLink.vue'
import type { InvoiceLine } from '../../../types/finance'
import { makeInvoice } from './fixtures'

const line = (over: Partial<InvoiceLine>): InvoiceLine => ({
  id: 'l', invoice_id: 'inv-1', sort_order: 0, description: 'DJ set', quantity: 1, unit_minor: 10000, unit_code: 'C62',
  tax_bps: 1900, line_total_minor: 10000, created_at: '2026-09-25T10:00:00Z', ...over,
})

afterEach(() => vi.unstubAllGlobals())

describe('InvoiceReferences', () => {
  it('lists only the references that are set, with the Leitweg-ID label', () => {
    const w = mount(InvoiceReferences, { props: { invoice: makeInvoice({ buyer_reference: '04011000-12345-34', payment_terms: 'Payable within 14 days' }) } })
    expect(w.text()).toContain('BUYER REFERENCE (LEITWEG-ID)')
    expect(w.text()).toContain('04011000-12345-34')
    expect(w.text()).toContain('Payable within 14 days')
    expect(w.text()).not.toContain('PURCHASE ORDER')
    expect(w.text()).not.toContain('CONTRACT REFERENCE')
  })

  it('renders nothing when there are no references or terms', () => {
    expect(mount(InvoiceReferences, { props: { invoice: makeInvoice() } }).html()).toBe('<!--v-if-->')
  })

  it('tolerates an invoice from an older API that lacks the fields', () => {
    const inv = makeInvoice() as unknown as Record<string, unknown>
    delete inv.buyer_reference
    expect(() => mount(InvoiceReferences, { props: { invoice: inv as never } })).not.toThrow()
  })
})

describe('InvoiceTotals units', () => {
  it('shows the unit next to the quantity, and nothing extra for single pieces', () => {
    const w = mount(InvoiceTotals, {
      props: {
        invoice: makeInvoice(),
        lines: [
          line({ id: 'a', description: 'DJ set', quantity: 3, unit_code: 'HUR', line_total_minor: 30000 }),
          line({ id: 'b', description: 'Booking fee', quantity: 1, unit_code: 'LS' }),
          line({ id: 'c', description: 'Merch', quantity: 5, unit_code: 'C62', line_total_minor: 50000 }),
          line({ id: 'd', description: 'Plain', quantity: 1, unit_code: 'C62' }),
        ],
      },
    })
    const rows = w.findAll('.tot-desc').map((r) => r.text().replace(/\s+/g, ' '))
    expect(rows).toEqual(['DJ set × 3 hour', 'Booking fee × 1 lump sum', 'Merch × 5', 'Plain'])
  })
})

describe('InvoicePdfLink', () => {
  it('links to the GET pdf endpoint as a download, without fetching it', () => {
    const fetchSpy = vi.fn()
    vi.stubGlobal('$fetch', fetchSpy)
    const w = mount(InvoicePdfLink, { props: { invoice: makeInvoice({ id: 'abc-123', invoice_number: 'INV-0007-EUR' }) } })
    const a = w.get('a')
    expect(a.attributes('href')).toBe('/api/v1/finance/invoices/abc-123/pdf')
    expect(a.attributes('download')).toBe('INV-0007-EUR.pdf')
    expect(a.text()).toBe('DOWNLOAD PDF')
    expect(a.attributes('aria-label')).toContain('INV-0007-EUR')
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('does not intercept the click outside the demo build', async () => {
    const w = mount(InvoicePdfLink, { props: { invoice: makeInvoice() } })
    const ev = new MouseEvent('click', { bubbles: true, cancelable: true })
    w.get('a').element.dispatchEvent(ev)
    expect(ev.defaultPrevented).toBe(false)
  })

  it('is offered for drafts too (the API renders them)', () => {
    const w = mount(InvoicePdfLink, { props: { invoice: makeInvoice({ status: 'draft', invoice_number: null }) } })
    expect(w.get('a').attributes('download')).toBe('DRAFT.pdf')
  })
})
