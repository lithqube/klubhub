import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import EarningsWorkspace from '../EarningsWorkspace.vue'
import EntryProfitLoss from '../EntryProfitLoss.vue'
import EntryList from '../EntryList.vue'
import { useEarningsStore } from '../../../stores/earnings'
import { FinanceApiError, useInvoiceStore } from '../../../stores/invoice'
import InvoiceDetailSheet from '../InvoiceDetailSheet.vue'
import InvoiceDraftEditor from '../InvoiceDraftEditor.vue'
import { makeInvoice } from './fixtures'
import { defineComponent, h } from 'vue'
import type { Entry } from '../../../types/finance'

const row = (over: Partial<Entry> = {}): Entry => ({ id: 'e', kind: 'income', amount_minor: 100, currency: 'EUR', category: 'gig_fee', entry_date: '2026-09-01', description: 'Fee', notes: '', gig_id: null, status: 'active', auto_generated: false, source_kind: 'manual', source_id: null, source_amount_minor: null, source_currency: null, source_description: '', created_at: 'T1', updated_at: 'T1', deleted_at: null, attachment_count: 0, ...over })
// Narrow the transport mock signature; Nitro's recursive route overloads are not the test contract.
const transport = () => vi.mocked($fetch as unknown as (url: string, options?: { method?: string; params?: unknown; body?: unknown }) => Promise<unknown>)
function deferred<T>() { let resolve!: (v: T) => void; let reject!: (e: unknown) => void; const promise = new Promise<T>((a, b) => { resolve = a; reject = b }); return { promise, resolve, reject } }
let w: VueWrapper | undefined
beforeEach(() => { setActivePinia(createPinia()); vi.stubGlobal('$fetch', vi.fn()); document.body.replaceChildren() })
afterEach(() => { w?.unmount(); w = undefined; vi.unstubAllGlobals(); vi.restoreAllMocks() })

it('edit categories follow the entry snapshot rather than the create preset', async () => {
  const s = useEarningsStore(); s.entries = [row({ kind: 'expense', category: 'gear', description: '', notes: 'Cable' })]; s.openEdit('e')
  w = mount(EarningsWorkspace, { attachTo: document.body }); await flushPromises()
  const values = [...document.querySelectorAll<HTMLSelectElement>('#ee-category option')].map(o => o.value)
  expect(values).toContain('gear'); expect(values).not.toContain('gig_fee')
})

it.each(['resolve', 'reject', 'switch', 'unmount'] as const)('entry lifetime fences old save: %s', async (mode) => {
  const s = useEarningsStore(); s.entries = [row()]; s.openEdit('e')
  w = mount(EarningsWorkspace, { attachTo: document.body }); await flushPromises()
  const old = deferred<unknown>(); const next = deferred<unknown>()
  transport().mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise)
  const submit = () => document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  submit(); await flushPromises()
  const cancel = [...document.querySelectorAll('button')].find(b => b.textContent?.trim() === 'CANCEL')!; cancel.click(); await flushPromises()
  if (mode === 'unmount') { w.unmount(); old.resolve({ data: row() }); await flushPromises(); expect(w.emitted('saved')).toBeUndefined(); w = undefined; return }
  if (mode === 'switch') s.entries.push(row({ id: 'other' }))
  s.openEdit(mode === 'switch' ? 'other' : 'e'); await flushPromises(); submit(); await flushPromises()
  expect(transport()).toHaveBeenCalledTimes(2)
  if (mode === 'reject') old.reject(new Error('obsolete failure'))
  else old.resolve({ data: row({ updated_at: 'T2' }) })
  await flushPromises()
  expect(s.createOpen).toBe(true)
  expect((document.querySelector('button[type=submit]') as HTMLButtonElement).disabled).toBe(true)
  expect(w.emitted('saved')).toBeUndefined()
  next.resolve({ data: row({ updated_at: 'T3' }) }); await flushPromises()
  expect(s.createOpen).toBe(false)
})

it.each(['success', 'unavailable'] as const)('latest entry filter owns rows, snapshot, errors and loading: %s', async (outcome) => {
  const s = useEarningsStore(); s.entries = [row()]; s.listLoaded = true
  w = mount(EntryList)
  const a = deferred<unknown>(); const b = deferred<unknown>()
  transport().mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise)
  const filter = { currency: 'EUR' }; const first = s.fetchEntries(filter); filter.currency = 'GBP'
  const second = s.fetchEntries({ currency: 'USD' })
  if (outcome === 'success') a.resolve({ data: [row()] })
  else a.reject(new FinanceApiError(503, 'unavailable', 'Old unavailable'))
  await first
  expect(s.listLoading).toBe(true); expect(s.listError).toBeNull(); expect(s.disabled).toBe(false)
  b.resolve({ data: [row({ currency: 'USD' })] }); await second; await flushPromises()
  expect(s.filterSnapshot).toEqual({ currency: 'USD' }); expect(s.entries[0]?.currency).toBe('USD')
  expect(s.listLoading).toBe(false)
})

it.each(['delete', 'put', 'failed-delete'] as const)('stale entry GET/list cannot undo record operation: %s', async (operation) => {
  const s = useEarningsStore(); s.entries = [row()]; s.listLoaded = true; w = mount(EntryList)
  const get = deferred<unknown>(); const list = deferred<unknown>(); const mutation = deferred<unknown>()
  transport().mockReturnValueOnce(get.promise).mockReturnValueOnce(list.promise).mockReturnValueOnce(mutation.promise)
  const pendingGet = s.fetchEntry('e'); const pendingList = s.fetchEntries()
  const write = operation === 'put' ? s.updateEntry('e', { ...row(), description: 'New', updated_at: 'T1' }) : s.deleteEntry('e', { updated_at: 'T1' })
  const writeResult = write.catch(e => e)
  if (operation === 'failed-delete') {
    list.resolve({ data: [] }); await pendingList
    expect(s.entries.map(e => e.id)).toEqual(['e'])
    mutation.reject(new Error('Delete failed')); await writeResult
    expect(s.entries.map(e => e.id)).toEqual(['e'])
  } else {
    mutation.resolve(operation === 'put' ? { data: row({ description: 'New', updated_at: 'T2' }) } : {})
    await writeResult
    list.resolve({ data: [row()] }); await pendingList
  }
  get.resolve({ data: row() }); await pendingGet; await flushPromises()
  if (operation === 'delete') expect(s.entries).toEqual([])
  if (operation === 'put') expect(s.entries[0]?.description).toBe('New')
})

it('GET started during a pending PUT cannot replace its successful confirmation', async () => {
  const s = useEarningsStore(); s.entries = [row()]; s.listLoaded = true; w = mount(EntryList)
  const put = deferred<unknown>(); const get = deferred<unknown>()
  transport().mockReturnValueOnce(put.promise).mockReturnValueOnce(get.promise)
  const save = s.updateEntry('e', { ...row(), description: 'Confirmed' }); const read = s.fetchEntry('e')
  put.resolve({ data: row({ description: 'Confirmed', updated_at: 'T2' }) }); await save
  get.resolve({ data: row() }); await read
  expect(s.entries[0]?.description).toBe('Confirmed')
})

it('late void cannot resurrect a confirmed deleted entry', async () => {
  const s = useEarningsStore(); s.entries = [row()]; s.listLoaded = true; w = mount(EntryList)
  const pending = deferred<unknown>(); transport().mockReturnValueOnce(pending.promise).mockResolvedValueOnce({})
  const voided = s.voidEntry('e', { updated_at: 'T1' }); await s.deleteEntry('e', { updated_at: 'T1' })
  pending.resolve({ data: row({ status: 'voided' }) }); await voided
  expect(s.entries).toEqual([])
})

const slots = defineComponent({ setup(_, { slots }) { return () => h('div', slots.default?.()) } })
it.each(['success', 'conflict'] as const)('late draft save cannot reseed or refetch another selected invoice: %s', async outcome => {
  const s = useInvoiceStore()
  const draft = (id: string) => makeInvoice({ id, status: 'draft', customer: { ...makeInvoice().customer, country: 'DE' } })
  s.current = draft('a')
  const Parent = defineComponent({ setup() { return () => s.current ? h(InvoiceDraftEditor, { invoice: s.current }) : null } })
  w = mount(Parent, { global: { stubs: { InvoicePartyFields: true, InvoiceTaxFields: true, InvoiceIssueChecklist: true } } })
  await w.find('textarea').setValue('A local')
  const pending = deferred<unknown>(); transport().mockReturnValueOnce(pending.promise).mockResolvedValue({ data: draft('a') })
  await w.find('form').trigger('submit'); await flushPromises()
  s.current = draft('b'); await flushPromises(); await w.find('textarea').setValue('B local')
  if (outcome === 'success') pending.resolve({ data: draft('a') })
  else pending.reject(new FinanceApiError(409, 'conflict', 'A obsolete conflict'))
  await flushPromises()
  expect(s.current?.id).toBe('b'); expect((w.find('textarea').element as HTMLTextAreaElement).value).toBe('B local')
  expect(transport().mock.calls).toHaveLength(1)
})
function detail() {
  useInvoiceStore().gigsLoaded = true
  return mount(InvoiceDetailSheet, { global: { stubs: { Sheet: slots, DialogTitle: slots, DialogDescription: slots, InvoiceDraftEditor: true, InvoiceTotals: true, InvoicePartySummary: true, InvoiceBalance: true, PaymentLedger: true, InvoiceActionsMenu: true, InvoiceConfirmDialog: true, NuxtLink: slots } } })
}
it.each(['success', 'unavailable', 'same-id'] as const)('detail selection lifetime fences obsolete invoice completion: %s', async (outcome) => {
  const s = useInvoiceStore(); w = detail()
  const a = deferred<unknown>(); const b = deferred<unknown>()
  transport().mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise).mockResolvedValue({ data: { ready: true, problems: [] } })
  const first = s.openDetail('a')
  if (outcome === 'same-id') s.setDetailOpen(false)
  const id = outcome === 'same-id' ? 'a' : 'b'; const second = s.openDetail(id)
  if (outcome === 'unavailable') a.reject(new FinanceApiError(503, 'unavailable', 'Old unavailable'))
  else a.resolve({ data: makeInvoice({ id: 'a', status: 'draft' }), lines: [] })
  await first; await flushPromises()
  expect(s.current).toBeNull(); expect(s.currentLoading).toBe(true); expect(s.currentError).toBeNull(); expect(s.disabled).toBe(false)
  b.resolve({ data: makeInvoice({ id, status: 'draft', internal_notes: 'Latest' }), lines: [] }); await second
  expect(s.current?.internal_notes).toBe('Latest'); expect(s.currentLoading).toBe(false)
})
it.each(['draft', 'issued'] as const)('late detail child %s cannot overwrite same-ID reopened detail', async (status) => {
  const s = useInvoiceStore(); w = detail()
  const oldChild = deferred<unknown>(); const newChild = deferred<unknown>()
  const invoice = makeInvoice({ id: 'a', status })
  transport().mockResolvedValueOnce({ data: invoice }).mockReturnValueOnce(oldChild.promise)
  const first = s.openDetail('a'); await flushPromises(); s.setDetailOpen(false)
  transport().mockResolvedValueOnce({ data: invoice }).mockReturnValueOnce(newChild.promise)
  const second = s.openDetail('a'); await flushPromises()
  oldChild.resolve({ data: status === 'draft' ? { ready: false, problems: [] } : [{ id: 'old' }] }); await first
  expect(s.currentLoading).toBe(true)
  if (status === 'draft') { expect(s.issueCheck).toBeNull(); expect(s.issueCheckLoading).toBe(true) }
  else expect(s.payments).toEqual([])
  newChild.resolve({ data: status === 'draft' ? { ready: true, problems: [] } : [] }); await second
  if (status === 'draft') expect(s.issueCheck?.ready).toBe(true)
  expect(s.currentLoading).toBe(false)
})

it('P&L year options retain modern years after selecting 2020', async () => {
  transport().mockResolvedValue({ data: {} })
  w = mount(EntryProfitLoss); await flushPromises()
  const year = w.find('select[aria-label=Year]'); const modern = String(new Date().getUTCFullYear())
  await year.setValue('2020'); await flushPromises()
  expect(year.findAll('option').map(o => o.attributes('value'))).toContain(modern)
  await year.setValue(modern); await flushPromises()
  expect(useEarningsStore().profitLossScope).toMatchObject({ year: Number(modern) })
})

it.each(['issue', 'cancel', 'pay', 'credit', 'correct'] as const)('invoice %s confirmation uses caller version snapshot, not refreshed cache', async (action) => {
  const s = useInvoiceStore(); const initial = makeInvoice({ id: 'a', updated_at: 'T1', status: 'issued' })
  s.detailOpen = true; s.detailId = 'a'; s.current = initial; s.invoices = [initial]; w = detail()
  vi.useFakeTimers()
  try {
    w.findComponent({ name: 'InvoiceActionsMenu' }).vm.$emit('action', action)
    await vi.runAllTimersAsync(); await flushPromises()
    s.current = { ...initial, updated_at: 'T2' }; s.invoices = [s.current]
    transport().mockRejectedValue(new Error('Stop after body'))
    w.findComponent({ name: 'InvoiceConfirmDialog' }).vm.$emit('confirm', 'Reason'); await flushPromises()
    expect(transport().mock.calls.find(([,o]) => o?.method === 'POST')?.[1]).toMatchObject({ body: { updated_at: 'T1' } })
  } finally { vi.useRealTimers() }
})

it('late confirmed invoice transition must not reopen or replace new detail selection', async () => {
  const s = useInvoiceStore(); s.detailOpen = true; s.detailId = 'a'; s.current = makeInvoice({ id: 'a' }); w = detail()
  const write = deferred<unknown>(); const next = deferred<unknown>()
  transport().mockReturnValueOnce(write.promise).mockReturnValueOnce(next.promise).mockResolvedValue({ data: [] })
  const action = s.cancelInvoice('a', s.current.updated_at)
  const selection = s.openDetail('b')
  write.resolve({ data: makeInvoice({ id: 'a', status: 'cancelled' }) }); await action
  expect(s.current).toBeNull(); expect(s.currentLoading).toBe(true)
  next.resolve({ data: makeInvoice({ id: 'b' }) }); await selection
  expect(s.current?.id).toBe('b')
})

it.each(['success', 'unavailable'] as const)('late payment mutation cannot fetch or overwrite a new invoice selection: %s', async outcome => {
  const s = useInvoiceStore(); s.detailOpen = true; s.detailId = 'a'; s.current = makeInvoice({ id: 'a' }); w = detail()
  const write = deferred<unknown>(); const next = deferred<unknown>()
  transport().mockReturnValueOnce(write.promise).mockReturnValueOnce(next.promise).mockResolvedValue({ data: [] })
  const action = s.createPayment('a', { amount_minor: 100, kind: 'payment', received: false }).catch(e => e)
  const selection = s.openDetail('b')
  if (outcome === 'unavailable') write.reject(new FinanceApiError(503, 'unavailable', 'Obsolete unavailable'))
  else write.resolve({ data: { id: 'payment', invoice_id: 'a' } })
  await action
  expect(s.current).toBeNull(); expect(s.currentLoading).toBe(true); expect(s.disabled).toBe(false)
  expect(transport().mock.calls.filter(([u]) => String(u).endsWith('/a'))).toHaveLength(0)
  next.resolve({ data: makeInvoice({ id: 'b' }) }); await selection
  expect(s.current?.id).toBe('b')
})

it.each(['reopen', 'switch', 'unmount'] as const)('entry reload respects dialog lifetime: %s', async (mode) => {
  const s = useEarningsStore(); s.entries = [row(), row({ id: 'other', description: 'Other' })]; s.openEdit('e')
  w = mount(EarningsWorkspace, { attachTo: document.body }); await flushPromises()
  transport().mockRejectedValueOnce(new FinanceApiError(409, 'conflict', 'Changed')).mockResolvedValueOnce({ data: row({ updated_at: 'T2' }) })
  document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); await flushPromises()
  const reload = deferred<unknown>(); transport().mockReturnValueOnce(reload.promise)
  ;[...document.querySelectorAll('button')].find(b => /DISCARD.*RELOAD/.test(b.textContent ?? ''))!.click(); await flushPromises()
  if (mode === 'unmount') { w.unmount(); reload.resolve({ data: row({ description: 'Remote' }) }); await flushPromises(); w = undefined; return }
  s.setCreateOpen(false); await flushPromises(); s.openEdit(mode === 'switch' ? 'other' : 'e'); await flushPromises()
  const description = document.querySelector('#ee-description') as HTMLInputElement
  description.value = 'New local'; description.dispatchEvent(new Event('input', { bubbles: true }))
  const next = deferred<unknown>(); transport().mockReturnValueOnce(next.promise)
  await flushPromises(); document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); await flushPromises()
  reload.resolve({ data: row({ description: 'Remote', updated_at: 'T3' }) }); await flushPromises()
  expect(description.value).toBe('New local'); expect(s.createOpen).toBe(true)
  expect((document.querySelector('button[type=submit]') as HTMLButtonElement).disabled).toBe(true)
  next.resolve({ data: row({ id: mode === 'switch' ? 'other' : 'e', description: 'New local' }) }); await flushPromises()
})

