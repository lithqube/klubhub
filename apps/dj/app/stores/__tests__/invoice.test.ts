import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import {
  useInvoiceStore,
  toFinanceError,
  FinanceApiError,
  CONFLICT_NOTICE,
  BAD_STATE_NOTICE,
  DISABLED_MESSAGE,
} from '../invoice'
import type { Invoice, Payment } from '../../types/finance'
import { emptyParty } from '../../utils/invoiceDisplay'

function makeInvoice(overrides: Partial<Invoice> = {}): Invoice {
  return {
    id: 'inv-1',
    kind: 'invoice',
    gig_id: 'gig-1',
    credits_invoice_id: null,
    replaced_by_invoice_id: null,
    invoice_number: null,
    number_prefix: 'INV',
    number_seq: null,
    currency: 'EUR',
    status: 'draft',
    supply_date: '2026-09-20',
    issued_at: null,
    due_at: '2026-10-04T00:00:00Z',
    paid_at: null,
    payment_ref: '',
    internal_notes: '',
    customer: { ...emptyParty(), legal_name: 'Club Alpha' },
    billing_profile: null,
    vat_treatment: 'domestic',
    tax_rate_bps: 1900,
    tax_note: '',
    subtotal_minor: 100000,
    tax_minor: 19000,
    total_minor: 119000,
    withholding_rate_bps: 0,
    withholding_minor: 0,
    net_payable_minor: 119000,
    tax_breakdown: [],
    received_minor: 0,
    pending_minor: 0,
    outstanding_minor: 0,
    updated_at: '2026-09-25T10:00:00Z',
    created_at: '2026-09-25T10:00:00Z',
    ...overrides,
  }
}

function httpError(status: number, body: unknown) {
  return Object.assign(new Error(`HTTP ${status}`), { statusCode: status, data: body })
}

const fetchMock = vi.fn()

describe('toFinanceError', () => {
  it('maps the error body code and problems', () => {
    const err = toFinanceError(httpError(422, { error: 'not_issuable', message: 'nope', problems: [{ field: 'customer.city', message: 'required' }] }))
    expect(err).toBeInstanceOf(FinanceApiError)
    expect(err.code).toBe('not_issuable')
    expect(err.problems).toHaveLength(1)
  })

  it('maps 503 to unavailable with the fixed message', () => {
    const err = toFinanceError(httpError(503, { error: 'unavailable', message: 'finance not configured' }))
    expect(err.code).toBe('unavailable')
    expect(err.message).toBe(DISABLED_MESSAGE)
  })

  it('unwraps an h3 envelope and extracts the 400 field', () => {
    const err = toFinanceError(httpError(400, { statusCode: 400, data: { error: 'validation_failed', message: 'validation failed: customer.country: must be ISO-2' } }))
    expect(err.code).toBe('validation_failed')
    expect(err.field).toBe('customer.country')
  })

  it('treats a missing status as a network error', () => {
    expect(toFinanceError(new Error('boom')).code).toBe('network')
  })
})

describe('useInvoiceStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
    // @ts-expect-error - mocking global $fetch
    global.$fetch = fetchMock
  })

  it('fetchInvoices loads the list and tolerates null data', async () => {
    fetchMock.mockResolvedValueOnce({ data: [makeInvoice()] })
    const store = useInvoiceStore()
    await store.fetchInvoices()
    expect(store.invoices).toHaveLength(1)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/invoices', { params: {} })

    fetchMock.mockResolvedValueOnce({ data: null })
    await store.fetchInvoices()
    expect(store.invoices).toEqual([])
  })

  it('flags disabled on 503', async () => {
    fetchMock.mockRejectedValueOnce(httpError(503, { error: 'unavailable', message: 'x' }))
    const store = useInvoiceStore()
    await store.fetchInvoices()
    expect(store.disabled).toBe(true)
    expect(store.listError?.code).toBe('unavailable')
  })

  it('filters by derived status (overdue, void)', async () => {
    fetchMock.mockResolvedValueOnce({
      data: [
        makeInvoice({ id: 'a', status: 'issued', due_at: '2000-01-01T00:00:00Z' }),
        makeInvoice({ id: 'b', status: 'issued', due_at: '2999-01-01T00:00:00Z' }),
        makeInvoice({ id: 'c', status: 'credited' }),
        makeInvoice({ id: 'd', status: 'draft' }),
      ],
    })
    const store = useInvoiceStore()
    await store.fetchInvoices()
    store.setFilter('overdue')
    expect(store.filteredInvoices.map((i) => i.id)).toEqual(['a'])
    store.setFilter('void')
    expect(store.filteredInvoices.map((i) => i.id)).toEqual(['c'])
    expect(store.filterCounts.all).toBe(4)
    expect(store.filterCounts.issued).toBe(1)
  })

  it('fetchInvoice loads lines and the issue check for drafts', async () => {
    fetchMock
      .mockResolvedValueOnce({ data: makeInvoice(), lines: [{ id: 'l1' }] })
      .mockResolvedValueOnce({ data: { ready: false, problems: [{ field: 'customer.city', message: 'required' }] } })
    const store = useInvoiceStore()
    await store.fetchInvoice('inv-1')
    expect(store.current?.id).toBe('inv-1')
    expect(store.lines).toHaveLength(1)
    expect(store.issueCheck?.problems[0]?.field).toBe('customer.city')
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/finance/invoices/inv-1/issue-check', {})
  })

  it('fetchInvoice loads payments for issued invoices', async () => {
    fetchMock
      .mockResolvedValueOnce({ data: makeInvoice({ status: 'issued' }), lines: [] })
      .mockResolvedValueOnce({ data: [{ id: 'p1' }] })
    const store = useInvoiceStore()
    await store.fetchInvoice('inv-1')
    expect(store.payments).toHaveLength(1)
  })

  it('updateInvoice sends the updated_at token from the server copy', async () => {
    const store = useInvoiceStore()
    store.current = makeInvoice()
    fetchMock
      .mockResolvedValueOnce({ data: makeInvoice({ updated_at: '2026-09-25T11:00:00Z' }) })
      .mockResolvedValueOnce({ data: { ready: true, problems: [] } })
    const inv = store.current!
    await store.updateInvoice('inv-1', {
      customer: inv.customer,
      vat_treatment: 'domestic',
      tax_rate_bps: 1900,
      tax_note: '',
      withholding_rate_bps: 0,
      supply_date: inv.supply_date,
      due_at: inv.due_at,
      number_prefix: 'INV',
      internal_notes: '',
    })
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/finance/invoices/inv-1')
    expect(opts.method).toBe('PUT')
    expect(opts.body.updated_at).toBe('2026-09-25T10:00:00Z')
    expect(store.current?.updated_at).toBe('2026-09-25T11:00:00Z')
    expect(store.issueCheck?.ready).toBe(true)
  })

  it('on 409 conflict refetches, sets the banner and rethrows (no retry)', async () => {
    const store = useInvoiceStore()
    store.current = makeInvoice()
    fetchMock
      .mockRejectedValueOnce(httpError(409, { error: 'conflict', message: 'stale' }))
      .mockResolvedValueOnce({ data: makeInvoice({ updated_at: '2026-09-25T12:00:00Z' }), lines: [] })
      .mockResolvedValueOnce({ data: { ready: true, problems: [] } })
    await expect(store.issueInvoice('inv-1')).rejects.toMatchObject({ code: 'conflict' })
    expect(store.notice).toBe(CONFLICT_NOTICE)
    expect(store.current?.updated_at).toBe('2026-09-25T12:00:00Z')
    // one failed POST, one GET, one issue-check — never a second POST
    expect(fetchMock.mock.calls.filter(([, o]) => o?.method === 'POST')).toHaveLength(1)
  })

  it('on 409 bad_state sets the bad-state banner', async () => {
    const store = useInvoiceStore()
    store.current = makeInvoice({ status: 'issued' })
    fetchMock
      .mockRejectedValueOnce(httpError(409, { error: 'bad_state', message: 'not draft' }))
      .mockResolvedValueOnce({ data: makeInvoice({ status: 'paid' }), lines: [] })
      .mockResolvedValueOnce({ data: [] })
    await expect(store.cancelInvoice('inv-1')).rejects.toMatchObject({ code: 'bad_state' })
    expect(store.notice).toBe(BAD_STATE_NOTICE)
    expect(store.current?.status).toBe('paid')
  })

  it('on 422 not_issuable shows the problems in the checklist', async () => {
    const store = useInvoiceStore()
    store.current = makeInvoice()
    fetchMock.mockRejectedValueOnce(
      httpError(422, { error: 'not_issuable', message: 'x', problems: [{ field: 'customer.vat_id', message: 'required' }] }),
    )
    await expect(store.issueInvoice('inv-1')).rejects.toMatchObject({ code: 'not_issuable' })
    expect(store.issueCheck).toEqual({ ready: false, problems: [{ field: 'customer.vat_id', message: 'required' }] })
  })

  it('createInvoice posts the minimal body and adds the draft to the list', async () => {
    const store = useInvoiceStore()
    fetchMock.mockResolvedValueOnce({ data: makeInvoice({ id: 'new' }) })
    const inv = await store.createInvoice({ gig_id: 'gig-1', due_at: '2026-10-04T00:00:00Z' })
    expect(inv.id).toBe('new')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/invoices', {
      method: 'POST',
      body: { gig_id: 'gig-1', due_at: '2026-10-04T00:00:00Z' },
    })
    expect(store.invoices[0]?.id).toBe('new')
    expect(store.activeInvoiceByGig['gig-1']?.id).toBe('new')
  })

  it('correctInvoice switches current to the replacement draft', async () => {
    const store = useInvoiceStore()
    store.current = makeInvoice({ status: 'issued', invoice_number: 'INV-0001-EUR' })
    const replacement = makeInvoice({ id: 'repl', status: 'draft' })
    fetchMock
      .mockResolvedValueOnce({
        data: {
          original: makeInvoice({ status: 'corrected', replaced_by_invoice_id: 'repl' }),
          credit_note: makeInvoice({ id: 'cn', kind: 'credit_note', status: 'issued', credits_invoice_id: 'inv-1' }),
          replacement,
        },
      })
      .mockResolvedValueOnce({ data: replacement, lines: [] })
      .mockResolvedValueOnce({ data: { ready: true, problems: [] } })
    const res = await store.correctInvoice('inv-1', 'wrong address')
    expect(res.replacement.id).toBe('repl')
    expect(store.current?.id).toBe('repl')
    expect(fetchMock.mock.calls[0]![1].body).toEqual({ reason: 'wrong address', updated_at: '2026-09-25T10:00:00Z' })
  })

  it('createPayment with received=true completes the pending payment', async () => {
    const store = useInvoiceStore()
    store.current = makeInvoice({ status: 'issued' })
    const pending: Payment = {
      id: 'p1', invoice_id: 'inv-1', currency: 'EUR', amount_minor: 5000, kind: 'deposit', status: 'pending',
      method: '', reference: '', received_at: null, created_at: 'x', updated_at: 'u1',
    }
    fetchMock
      .mockResolvedValueOnce({ data: pending })
      .mockResolvedValueOnce({ data: { ...pending, status: 'completed' } })
      .mockResolvedValueOnce({ data: store.current, lines: [] })
      .mockResolvedValueOnce({ data: [] })
    await store.createPayment('inv-1', { kind: 'deposit', amount_minor: 5000, received: true, received_at: '2026-09-25T00:00:00Z' })
    expect(fetchMock.mock.calls[0]![1].body).toMatchObject({ currency: 'EUR', amount_minor: 5000, kind: 'deposit' })
    const [putUrl, putOpts] = fetchMock.mock.calls[1]!
    expect(putUrl).toBe('/api/v1/finance/payments/p1')
    expect(putOpts.body).toMatchObject({ status: 'completed', updated_at: 'u1', received_at: '2026-09-25T00:00:00Z' })
  })

  it('createPayment on exceeds_balance refreshes and rethrows', async () => {
    const store = useInvoiceStore()
    store.current = makeInvoice({ status: 'issued' })
    fetchMock
      .mockRejectedValueOnce(httpError(422, { error: 'exceeds_balance', message: 'too much' }))
      .mockResolvedValueOnce({ data: store.current, lines: [] })
      .mockResolvedValueOnce({ data: [] })
    await expect(store.createPayment('inv-1', { kind: 'payment', amount_minor: 1 })).rejects.toMatchObject({
      code: 'exceeds_balance',
      message: 'too much',
    })
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })

  it('suggestTax sends customer country, VAT ID and business flag', async () => {
    const store = useInvoiceStore()
    fetchMock.mockResolvedValueOnce({ data: { vat_treatment: 'reverse_charge', tax_rate_bps: 0, tax_note: 'n', reason: 'r' } })
    const s = await store.suggestTax({ country: 'FR', vat_id: 'FR123', is_business: true })
    expect(s.vat_treatment).toBe('reverse_charge')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/invoices/tax-suggestion', {
      params: { customer_country: 'FR', customer_vat_id: 'FR123', customer_is_business: 'true' },
    })
  })

  it('fetchGigOptions maps gigs and reports failure without throwing', async () => {
    const store = useInvoiceStore()
    fetchMock.mockResolvedValueOnce([{ id: 'g1', date: '2026-09-20T00:00:00Z', venue: 'Club Alpha', status: 'played', fee_amount: '1500', fee_currency: 'EUR' }])
    await store.fetchGigOptions()
    expect(store.gigOptions[0]).toMatchObject({ id: 'g1', label: 'Club Alpha', fee_amount: 1500 })

    fetchMock.mockRejectedValueOnce(new Error('offline'))
    await store.fetchGigOptions(true)
    expect(store.gigsError).toBe('Could not load gigs.')
  })
})
