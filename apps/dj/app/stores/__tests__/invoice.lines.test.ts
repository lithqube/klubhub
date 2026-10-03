import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { FinanceApiError, invoicePdfUrl, toFinanceError, useInvoiceStore } from '../invoice'
import type { Invoice, InvoiceLine, InvoiceUpdateInput } from '../../types/finance'
import { makeInvoice } from '../../components/finance/__tests__/fixtures'

const fetchMock = vi.fn()
const httpError = (status: number, body: unknown) => Object.assign(new Error(`HTTP ${status}`), { statusCode: status, data: body })

function lineOf(over: Partial<InvoiceLine> = {}): InvoiceLine {
  return {
    id: 'l1', invoice_id: 'inv-1', sort_order: 0, description: 'DJ set', quantity: 1, unit_minor: 80000,
    unit_code: 'C62', tax_bps: 1900, line_total_minor: 80000, created_at: '2026-09-25T10:00:00Z', ...over,
  }
}

function input(over: Partial<InvoiceUpdateInput> = {}): InvoiceUpdateInput {
  const inv = makeInvoice({ status: 'draft' })
  return {
    customer: inv.customer, vat_treatment: 'domestic', tax_rate_bps: 1900, tax_note: '', withholding_rate_bps: 0,
    supply_date: inv.supply_date, due_at: inv.due_at, number_prefix: 'INV', internal_notes: '',
    buyer_reference: '', purchase_order_ref: '', contract_ref: '', payment_terms: '', updated_at: inv.updated_at, ...over,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  fetchMock.mockReset()
  vi.stubGlobal('$fetch', fetchMock)
})

describe('updateInvoice with lines', () => {
  it('sends the lines array and the reference fields exactly as given', async () => {
    const store = useInvoiceStore()
    store.current = makeInvoice({ status: 'draft' })
    fetchMock.mockResolvedValue({ data: makeInvoice({ status: 'draft' }), lines: [] })
    const lines = [
      { description: 'DJ set', quantity: 2, unit_minor: 15000, unit_code: 'HUR' as const },
      { description: 'Travel', quantity: 120, unit_minor: 35 },
    ]
    await store.updateInvoice('inv-1', input({ lines, buyer_reference: '04011000-12345-34', payment_terms: 'Payable within 14 days' }))

    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/finance/invoices/inv-1')
    expect(opts.method).toBe('PUT')
    expect(opts.body.lines).toEqual(lines)
    expect(opts.body).toMatchObject({ buyer_reference: '04011000-12345-34', payment_terms: 'Payable within 14 days' })
  })

  it('leaves `lines` out of the body when the caller did not send any (the API then keeps them)', async () => {
    const store = useInvoiceStore()
    store.current = makeInvoice({ status: 'draft' })
    fetchMock.mockResolvedValue({ data: makeInvoice({ status: 'draft' }) })
    await store.updateInvoice('inv-1', input())
    expect(fetchMock.mock.calls[0]![1].body).not.toHaveProperty('lines')
    // No extra GET: nothing changed in the lines.
    expect(fetchMock.mock.calls.filter(([, o]) => !o?.method || o.method === 'GET').every(([u]) => u.endsWith('/issue-check'))).toBe(true)
  })

  it('reads the replaced lines back (the PUT only returns the invoice) and keeps totals from the PUT response', async () => {
    const store = useInvoiceStore()
    store.current = makeInvoice({ status: 'draft' })
    store.lines = [lineOf()]
    const saved: Invoice = makeInvoice({ status: 'draft', subtotal_minor: 4200, tax_minor: 798, total_minor: 4998, updated_at: '2026-09-25T11:00:00Z' })
    const stored = [lineOf({ id: 'n1', description: 'Travel', quantity: 120, unit_minor: 35, line_total_minor: 4200 })]
    fetchMock.mockImplementation(async (_url: string, o?: { method?: string }) => (o?.method === 'PUT' ? { data: saved } : { data: saved, lines: stored }))

    await store.updateInvoice('inv-1', input({ lines: [{ description: 'Travel', quantity: 120, unit_minor: 35 }] }))

    expect(store.current?.total_minor).toBe(4998)
    expect(store.lines).toEqual(stored)
  })

  it('tells the user when the lines could not be reloaded after a successful save', async () => {
    const store = useInvoiceStore()
    store.current = makeInvoice({ status: 'draft' })
    fetchMock
      .mockResolvedValueOnce({ data: makeInvoice({ status: 'draft', updated_at: '2026-09-25T11:00:00Z' }) })
      .mockRejectedValueOnce(httpError(500, {}))
      .mockResolvedValue({ data: { ready: true, problems: [] } })
    await store.updateInvoice('inv-1', input({ lines: [{ description: 'x', quantity: 1, unit_minor: 1 }] }))
    expect(store.current?.updated_at).toBe('2026-09-25T11:00:00Z')
    expect(store.notice).toMatch(/could not be reloaded/i)
  })

  it('does not change the lines or the invoice when the server rejects them', async () => {
    const store = useInvoiceStore()
    const before = makeInvoice({ status: 'draft' })
    store.current = before
    store.lines = [lineOf()]
    fetchMock.mockRejectedValue(httpError(400, { error: 'validation_failed', message: 'validation failed: lines[0].quantity: must be between 1 and 1000000' }))
    await expect(store.updateInvoice('inv-1', input({ lines: [{ description: 'x', quantity: 0, unit_minor: 1 }] }))).rejects.toMatchObject({
      code: 'validation_failed',
      field: 'lines[0].quantity',
    })
    expect(store.current).toEqual(before)
    expect(store.lines).toEqual([lineOf()])
  })
})

describe('FinanceApiError field parsing', () => {
  it('names the indexed line field, not just its last segment', () => {
    const e = toFinanceError(httpError(400, { error: 'validation_failed', message: 'validation failed: lines[2].description: required; lines[0].unit_code: must be one of C62' }))
    expect(e).toBeInstanceOf(FinanceApiError)
    expect(e.field).toBe('lines[2].description')
  })

  it('still names plain and dotted fields', () => {
    expect(toFinanceError(httpError(400, { error: 'validation_failed', message: 'validation failed: customer.country: must be 2 letters' })).field).toBe('customer.country')
    expect(toFinanceError(httpError(400, { error: 'validation_failed', message: 'validation failed: buyer_reference: exceeds 100 characters' })).field).toBe('buyer_reference')
  })
})

describe('invoicePdfUrl', () => {
  it('points at the GET pdf endpoint of the invoice', () => {
    expect(invoicePdfUrl('9f1c')).toBe('/api/v1/finance/invoices/9f1c/pdf')
  })
})
