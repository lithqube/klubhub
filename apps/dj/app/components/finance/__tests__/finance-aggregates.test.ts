import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import Finance from '../../../pages/finance.vue'
import { useEarningsStore } from '../../../stores/earnings'
import { FinanceApiError } from '../../../stores/invoice'
import type { Entry } from '../../../types/finance'
// Narrow the transport mock signature; Nitro's recursive route overloads are not the test contract.
const transport = () => vi.mocked($fetch as unknown as (url: string, options?: { method?: string; params?: unknown; body?: unknown }) => Promise<unknown>)
function deferred<T>() { let resolve!: (v: T) => void; let reject!: (e: unknown) => void; const promise = new Promise<T>((a, b) => { resolve = a; reject = b }); return { promise, resolve, reject } }
const entry: Entry = { id: 'e', kind: 'income', amount_minor: 100, currency: 'EUR', category: 'gig_fee', entry_date: '2026-09-01', description: 'Fee', notes: '', gig_id: null, status: 'active', auto_generated: false, source_kind: 'manual', source_id: null, source_amount_minor: null, source_currency: null, source_description: '', created_at: 'T1', updated_at: 'T1', deleted_at: null }
let w: VueWrapper
beforeEach(() => { setActivePinia(createPinia()); vi.stubGlobal('useHead', vi.fn()); vi.stubGlobal('$fetch', vi.fn()); document.body.replaceChildren() })
afterEach(() => { w?.unmount(); vi.unstubAllGlobals(); vi.restoreAllMocks() })
function page() { return mount(Finance, { attachTo: document.body, global: { stubs: { InvoiceList: true, InvoiceWorkspace: true, EntryReconciliationPanel: true, EntryReconciliationDialog: true } } }) }
function aggregateCalls() { return transport().mock.calls.filter(([u]) => /\/(summary|profit-loss)$/.test(String(u))).map(([u, o]) => ({ url: String(u), params: o?.params })) }

it.each(['create', 'edit', 'void', 'delete'] as const)('confirmed %s refreshes each aggregate exactly once, never before confirmation', async (operation) => {
  transport().mockImplementation(async (u) => ({ data: String(u).endsWith('/entries') ? [entry] : {} }))
  w = page(); await flushPromises()
  const s = useEarningsStore(); const baseline = aggregateCalls().length
  const write = deferred<unknown>()
  transport().mockImplementation((u, o) => o?.method ? write.promise : Promise.resolve({ data: {} }))
  if (operation === 'edit' || operation === 'create') {
    if (operation === 'edit') s.openEdit('e'); else s.openCreate()
    await flushPromises()
    if (operation === 'create') {
      const input = document.querySelector('#ee-amount') as HTMLInputElement; input.value = '1'; input.dispatchEvent(new Event('input', { bubbles: true }))
      const desc = document.querySelector('#ee-description') as HTMLInputElement; desc.value = 'Fee'; desc.dispatchEvent(new Event('input', { bubbles: true }))
    }
    await flushPromises()
    document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  } else {
    w.findComponent({ name: 'EntryList' }).vm.$emit(operation, entry); await flushPromises()
    const button = [...document.querySelectorAll<HTMLButtonElement>('.finance-confirm button')].find(b => b.textContent?.trim() === (operation === 'void' ? 'CONFIRM VOID' : 'DELETE'))!
    button.click(); button.click()
  }
  await flushPromises(); expect(aggregateCalls()).toHaveLength(baseline)
  expect(transport().mock.calls.filter(([,o]) => o?.method)).toHaveLength(1)
  write.resolve({ data: { ...entry, status: operation === 'void' ? 'voided' : 'active' } }); await flushPromises()
  const refreshes = aggregateCalls().slice(baseline)
  expect(refreshes).toHaveLength(2)
  expect(refreshes.map(c => c.url).sort()).toEqual(['/api/v1/finance/profit-loss', '/api/v1/finance/summary'])
})

it.each(['success', 'unavailable'] as const)('aggregate panes fence obsolete results and finally during current reads: %s', async (outcome) => {
  transport().mockImplementation(async (u) => ({ data: String(u).endsWith('/entries') ? [entry] : {} }))
  w = page(); await flushPromises(); const s = useEarningsStore()
  const oldSummary = deferred<unknown>(); const oldPL = deferred<unknown>(); const newSummary = deferred<unknown>(); const newPL = deferred<unknown>()
  transport().mockReturnValueOnce(oldSummary.promise).mockReturnValueOnce(oldPL.promise).mockReturnValueOnce(newSummary.promise).mockReturnValueOnce(newPL.promise)
  const scope = { from: '2020-01-01', to: '2021-01-01' }
  const a = s.refreshSummary(scope); scope.to = '2025-01-01'
  const b = s.refreshProfitLoss({ ...s.profitLossScope })
  const c = s.refreshSummary({ scope: 'month', year: new Date().getUTCFullYear(), month: new Date().getUTCMonth() + 1 })
  const d = s.refreshProfitLoss({ ...s.profitLossScope })
  if (outcome === 'success') { oldSummary.resolve({ data: { OLD: {} } }); oldPL.resolve({ data: { OLD: {} } }) }
  else { oldSummary.reject(new FinanceApiError(503, 'unavailable', 'Old unavailable')); oldPL.reject(new FinanceApiError(503, 'unavailable', 'Old unavailable')) }
  await Promise.all([a,b])
  expect(s.summaryLoading).toBe(true); expect(s.profitLossLoading).toBe(true); expect(s.disabled).toBe(false)
  expect(s.summaryError).toBeNull(); expect(s.profitLossError).toBeNull()
  const totals = { EUR: { currency: 'EUR', income_minor: 100, expense_minor: 0, profit_minor: 100 } }
  newSummary.resolve({ data: totals }); newPL.resolve({ data: totals }); await Promise.all([c,d]); await flushPromises()
  expect(s.summary).toEqual(totals); expect(s.profitLossByScope?.data).toEqual(totals)
})

it.each(['success', 'unavailable'] as const)('confirmed mutation supersedes pending aggregates even when old responses arrive last: %s', async (outcome) => {
  transport().mockImplementation(async u => ({ data: String(u).endsWith('/entries') ? [entry] : {} }))
  w = page(); await flushPromises(); const s = useEarningsStore()
  const oldSummary = deferred<unknown>(); const oldPL = deferred<unknown>(); const newSummary = deferred<unknown>(); const newPL = deferred<unknown>(); const write = deferred<unknown>()
  transport().mockReturnValueOnce(oldSummary.promise).mockReturnValueOnce(oldPL.promise).mockReturnValueOnce(write.promise).mockReturnValueOnce(newSummary.promise).mockReturnValueOnce(newPL.promise)
  const a = s.refreshSummary(); const b = s.refreshProfitLoss()
  s.openEdit('e'); await flushPromises()
  document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); await flushPromises()
  write.resolve({ data: { ...entry, amount_minor: 200, updated_at: 'T2' } }); await flushPromises()
  const totals = { EUR: { currency: 'EUR', income_minor: 200, expense_minor: 0, profit_minor: 200 } }
  newSummary.resolve({ data: totals }); newPL.resolve({ data: totals }); await flushPromises()
  expect(s.summary).toEqual(totals); expect(s.profitLossByScope?.data).toEqual(totals)
  if (outcome === 'success') { oldSummary.resolve({ data: {} }); oldPL.resolve({ data: {} }) }
  else { oldSummary.reject(new FinanceApiError(503, 'unavailable', 'Obsolete')); oldPL.reject(new FinanceApiError(503, 'unavailable', 'Obsolete')) }
  await Promise.all([a,b]); await flushPromises()
  expect(s.summary).toEqual(totals); expect(s.profitLossByScope?.data).toEqual(totals); expect(s.disabled).toBe(false)
  expect(s.summaryError).toBeNull(); expect(s.profitLossError).toBeNull()
})
it.each(['void', 'delete'] as const)('failed %s remains recoverable and accessible without refreshing aggregates', async operation => {
  transport().mockImplementation(async u => ({ data: String(u).endsWith('/entries') ? [entry] : {} }))
  w = page(); await flushPromises(); const baseline = aggregateCalls().length
  transport().mockRejectedValue(new FinanceApiError(400, 'validation_failed', 'Mutation rejected'))
  w.findComponent({ name: 'EntryList' }).vm.$emit(operation, entry); await flushPromises()
  const confirm = [...document.querySelectorAll<HTMLButtonElement>('.finance-confirm button')].find(b => b.textContent?.trim() === (operation === 'void' ? 'CONFIRM VOID' : 'DELETE'))!
  confirm.click(); await flushPromises()
  expect(w.find('[role=alert]').text()).toContain('Mutation rejected')
  expect(useEarningsStore().entries.map(e => e.id)).toContain('e'); expect(aggregateCalls()).toHaveLength(baseline)
  expect(document.querySelector('.finance-confirm')).not.toBeNull()
})
it('cancelled dialog confirmed save still refreshes finance totals without closing a reopened dialog', async () => {
  transport().mockImplementation(async u => ({ data: String(u).endsWith('/entries') ? [entry] : {} }))
  w = page(); await flushPromises(); const s = useEarningsStore(); const baseline = aggregateCalls().length
  const write = deferred<unknown>(); transport().mockImplementation((u, o) => o?.method ? write.promise : Promise.resolve({ data: {} }))
  s.openEdit('e'); await flushPromises(); document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); await flushPromises()
  s.setCreateOpen(false); await flushPromises(); s.openEdit('e'); await flushPromises()
  write.resolve({ data: { ...entry, amount_minor: 200, updated_at: 'T2' } }); await flushPromises()
  expect(s.createOpen).toBe(true); expect(aggregateCalls().slice(baseline)).toHaveLength(2)
})

it('current aggregate failures preserve cached totals and scope input snapshot', async () => {
  transport().mockImplementation(async u => ({ data: String(u).endsWith('/entries') ? [entry] : {} }))
  w = page(); await flushPromises(); const s = useEarningsStore()
  const totals = { EUR: { currency: 'EUR', income_minor: 200, expense_minor: 0 } }
  s.summary = totals
  const pending = deferred<unknown>(); transport().mockReturnValueOnce(pending.promise)
  const arg = { from: '2020-01-01', to: '2021-01-01' }; const read = s.refreshSummary(arg); arg.to = '2099-01-01'
  expect(s.summaryScope).toEqual({ from: '2020-01-01', to: '2021-01-01' })
  pending.reject(new FinanceApiError(400, 'validation_failed', 'Current failure')); await read; expect(s.summary).toEqual(totals); expect(s.summaryError?.message).toBe('Current failure')
})
