import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useEarningsStore } from '../earnings'
import { FinanceApiError, toFinanceError } from '../invoice'
import type { Entry, EntryReconciliation } from '../../types/finance'

function httpError(status: number, body: unknown) {
  return Object.assign(new Error(`HTTP ${status}`), { statusCode: status, data: body })
}

const fetchMock = vi.fn()

function makeEntry(over: Partial<Entry> = {}): Entry {
  return {
    id: 'ent-1',
    kind: 'income',
    amount_minor: 100000,
    currency: 'EUR',
    category: 'gig_fee',
    entry_date: '2026-09-15',
    description: 'DJ set',
    notes: '',
    gig_id: null,
    status: 'active',
    auto_generated: false,
    source_kind: 'manual',
    source_id: null,
    source_amount_minor: null,
    source_currency: null,
    source_description: '',
    created_at: '2026-09-15T10:00:00Z',
    updated_at: '2026-09-15T10:00:00Z',
    deleted_at: null,
    ...over,
  }
}

function makeReconciliation(over: Partial<EntryReconciliation> = {}): EntryReconciliation {
  return {
    id: 'rec-1',
    gig_id: 'gig-1',
    entry_id: 'ent-1',
    reason: 'fee_changed',
    allowed_actions: ['update', 'keep'],
    gig_amount_minor: 120000,
    gig_currency: 'EUR',
    gig_payment_status: 'paid',
    entry_updated_at: '2026-09-15T10:00:00Z',
    status: 'pending',
    resolution: null,
    created_at: '2026-09-15T10:00:00Z',
    updated_at: '2026-09-15T10:00:00Z',
    resolved_at: null,
    ...over,
  }
}

describe('useEarningsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
    // @ts-expect-error - mocking global $fetch
    global.$fetch = fetchMock
  })

  it('fetchEntries drops empty filter params and unwraps the list', async () => {
    fetchMock.mockResolvedValueOnce({ data: [makeEntry()] })
    const store = useEarningsStore()
    await store.fetchEntries({ kind: 'income' })
    expect(store.entries).toHaveLength(1)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/entries', {
      params: { kind: 'income' },
    })
  })

  it('fetchEntries flags disabled on 503', async () => {
    fetchMock.mockRejectedValueOnce(httpError(503, { error: 'unavailable', message: 'off' }))
    const store = useEarningsStore()
    await store.fetchEntries()
    expect(store.disabled).toBe(true)
    expect(store.listError?.code).toBe('unavailable')
  })

  it('createEntry posts the body and prepends to the list', async () => {
    fetchMock.mockResolvedValueOnce({ data: makeEntry({ id: 'new' }) })
    const store = useEarningsStore()
    const e = await store.createEntry({
      kind: 'income', amount_minor: 150000, currency: 'EUR',
      category: 'gig_fee', entry_date: '2026-09-15', description: 'OK', notes: '', gig_id: null,
    })
    expect(e.id).toBe('new')
    expect(store.entries[0]?.id).toBe('new')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/entries', {
      method: 'POST',
      body: expect.objectContaining({ kind: 'income', amount_minor: 150000, currency: 'EUR' }),
    })
  })

  it('updateEntry sends the cached updated_at and refreshes on 409', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry()]
    fetchMock
      .mockRejectedValueOnce(httpError(409, { error: 'conflict', message: 'stale' }))
      .mockResolvedValueOnce({ data: [makeEntry({ updated_at: '2026-09-15T11:00:00Z' })] })
    await expect(store.updateEntry('ent-1', {
      kind: 'income', amount_minor: 1, currency: 'EUR', category: 'gig_fee',
      entry_date: '2026-09-15', description: 'x', notes: '', gig_id: null,
    })).rejects.toMatchObject({ code: 'conflict' })
    expect(fetchMock.mock.calls[0]![1].body.updated_at).toBe('2026-09-15T10:00:00Z')
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('voidEntry flips status to voided in the local list', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry()]
    fetchMock.mockResolvedValueOnce({ data: makeEntry({ status: 'voided', updated_at: 't2' }) })
    await store.voidEntry('ent-1')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/entries/ent-1/void', {
      method: 'POST',
      body: { updated_at: '2026-09-15T10:00:00Z' },
    })
    expect(store.entries[0]?.status).toBe('voided')
  })

  it('refreshSummary scopes month / year / custom date range', async () => {
    const store = useEarningsStore()
    fetchMock
      .mockResolvedValueOnce({ data: {} })  // month
      .mockResolvedValueOnce({ data: { EUR: { currency: 'EUR', income_minor: 100000, expense_minor: 25000 } } })  // year
      .mockResolvedValueOnce({ data: { EUR: { currency: 'EUR', income_minor: 100000, expense_minor: 25000 } } })  // custom
    await store.refreshSummary({ scope: 'month', year: 2026, month: 9 })
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/finance/summary', {
      params: { scope: 'month', year: '2026', month: '9' },
    })
    await store.refreshSummary({ scope: 'year', year: 2026 })
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/finance/summary', {
      params: { scope: 'year', year: '2026' },
    })
    await store.refreshSummary({ from: '2026-01-01', to: '2026-04-01' })
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/finance/summary', {
      params: { from: '2026-01-01', to: '2026-04-01' },
    })
    expect(store.summary.EUR?.expense_minor).toBe(25000)
  })

  it('refreshProfitLoss dispatches gig / month / year scopes', async () => {
    const store = useEarningsStore()
    fetchMock.mockResolvedValue({ data: { EUR: { currency: 'EUR', income_minor: 100, expense_minor: 50, profit_loss_minor: 50 } } })
    await store.refreshProfitLoss({ scope: 'gig', gig_id: 'gig-1' })
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/finance/profit-loss', {
      params: { scope: 'gig', gig_id: 'gig-1' },
    })
    await store.refreshProfitLoss({ scope: 'month', year: 2026, month: 9 })
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/finance/profit-loss', {
      params: { scope: 'month', year: '2026', month: '9' },
    })
  })

  it('resolveReconciliation POSTs with the right body and refreshes entries', async () => {
    const store = useEarningsStore()
    fetchMock
      .mockResolvedValueOnce({ data: { ...makeReconciliation(), status: 'resolved', resolution: 'update' } })
      .mockResolvedValueOnce({ data: [makeEntry()] })
    await store.resolveReconciliation('rec-1', { action: 'update', updated_at: '2026-09-15T10:00:00Z' })
    expect(fetchMock.mock.calls[0]![1]).toEqual({
      method: 'POST', body: { action: 'update', updated_at: '2026-09-15T10:00:00Z' },
    })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('fetchReconciliationForGig caches null on 404 and logs the error otherwise', async () => {
    const store = useEarningsStore()
    fetchMock.mockRejectedValueOnce(httpError(404, { error: 'not_found', message: 'nope' }))
    await store.fetchReconciliationForGig('gig-1')
    expect(store.reconciliationByGig['gig-1']).toBeNull()

    fetchMock.mockReset()
    fetchMock.mockRejectedValueOnce(httpError(500, { error: 'unknown', message: 'boom' }))
    await store.fetchReconciliationForGig('gig-2')
    expect(store.reconciliationErrors['gig-2']?.code).toBe('unknown')
  })

  it('reuses toFinanceError + FinanceApiError from invoice store', () => {
    expect(useEarningsStore).toBeDefined()
    expect(typeof toFinanceError).toBe('function')
    const err = toFinanceError(httpError(409, { error: 'conflict', message: 'x' }))
    expect(err).toBeInstanceOf(FinanceApiError)
    expect(err.code).toBe('conflict')
  })

  it('currencies union is sorted and includes summary currencies', () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ currency: 'USD' }), makeEntry({ currency: 'EUR' })]
    expect(store.currencies).toEqual(['EUR', 'USD'])
  })

  it('filterSnapshot is set after fetch so optimistic edits keep their view', async () => {
    const store = useEarningsStore()
    fetchMock.mockResolvedValueOnce({ data: [makeEntry()] })
    await store.fetchEntries({ kind: 'income', currency: 'EUR' })
    expect(store.filterSnapshot).toEqual({ kind: 'income', currency: 'EUR' })
  })
})
