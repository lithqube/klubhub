// Review follow-ups for the finance hardening branch (real Pinia, mounted
// components, deferred transport; only the HTTP transport and leaf children
// are stubbed).
//   NEW-1: create must never POST `updated_at` (Go CreateEntryRequest uses
//          DisallowUnknownFields).
//   FE-1a: edit with an id missing from the cache must reload the entry or
//          refuse to save; never PUT a preset kind with an empty token.
//   FE-5 : a confirmed invoice write whose selection moved on still updates
//          the list row, but never `current`.
//   Plus: stale-GET-after-delete, delete 409, InvoiceDetailSheet lifetime,
//          InvoiceDraftEditor same-ID reopen.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import EarningsWorkspace from '../EarningsWorkspace.vue'
import InvoiceDetailSheet from '../InvoiceDetailSheet.vue'
import InvoiceDraftEditor from '../InvoiceDraftEditor.vue'
import Finance from '../../../pages/finance.vue'
import { useEarningsStore } from '../../../stores/earnings'
import { FinanceApiError, useInvoiceStore } from '../../../stores/invoice'
import { makeInvoice } from './fixtures'
import type { Entry } from '../../../types/finance'

const toastSpy = vi.hoisted(() => vi.fn())
vi.mock('#kui/components/ui/toast/use-toast', () => ({ useToast: () => ({ toast: toastSpy }) }))

type Call = [string, { method?: string; params?: unknown; body?: Record<string, unknown> }?]
const transport = () => vi.mocked($fetch as unknown as (url: string, options?: Call[1]) => Promise<unknown>)
const writes = (method: string) => transport().mock.calls.filter(([, o]) => o?.method === method)
const row = (over: Partial<Entry> = {}): Entry => ({ id: 'e', kind: 'income', amount_minor: 100, currency: 'EUR', category: 'gig_fee', entry_date: '2026-09-01', description: 'Fee', notes: '', gig_id: null, status: 'active', auto_generated: false, source_kind: 'manual', source_id: null, source_amount_minor: null, source_currency: null, source_description: '', created_at: 'T1', updated_at: 'T1', deleted_at: null, ...over })
function deferred<T>() { let resolve!: (v: T) => void; let reject!: (e: unknown) => void; const promise = new Promise<T>((a, b) => { resolve = a; reject = b }); return { promise, resolve, reject } }
async function type(selector: string, value: string): Promise<void> {
  const el = document.querySelector(selector) as HTMLInputElement | HTMLTextAreaElement
  expect(el).not.toBeNull()
  el.value = value; el.dispatchEvent(new Event('input', { bubbles: true })); await flushPromises()
}
async function submit(): Promise<void> {
  document.querySelector('form.ee-form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); await flushPromises()
}

let w: VueWrapper | undefined
beforeEach(() => { setActivePinia(createPinia()); vi.stubGlobal('useHead', vi.fn()); vi.stubGlobal('$fetch', vi.fn()); document.body.replaceChildren(); toastSpy.mockClear() })
afterEach(() => { w?.unmount(); w = undefined; vi.unstubAllGlobals(); vi.restoreAllMocks(); vi.useRealTimers() })

describe('NEW-1 create body', () => {
  it.each(['income', 'expense'] as const)('POST /entries for %s omits updated_at and keeps the contract keys', async (kind) => {
    const s = useEarningsStore(); s.openCreate(kind)
    w = mount(EarningsWorkspace, { attachTo: document.body }); await flushPromises()
    transport().mockImplementation(async (_u, o) => ({ data: row({ ...(o?.body as object), id: 'new' }) }))
    await type('#ee-amount', '12.50')
    await type(kind === 'income' ? '#ee-description' : '#ee-notes', 'Something')
    await submit()
    const post = writes('POST')
    expect(post).toHaveLength(1)
    const body = post[0]![1]!.body!
    expect('updated_at' in body).toBe(false)
    expect(Object.keys(body).sort()).toEqual(['amount_minor', 'category', 'currency', 'description', 'entry_date', 'gig_id', 'kind', 'notes'])
    expect(body).toMatchObject({ kind, amount_minor: 1250, currency: 'EUR', gig_id: null })
  })

  it('edit still sends the captured token', async () => {
    const s = useEarningsStore(); s.entries = [row()]; s.openEdit('e')
    w = mount(EarningsWorkspace, { attachTo: document.body }); await flushPromises()
    transport().mockResolvedValue({ data: row({ updated_at: 'T2' }) })
    await type('#ee-amount', '5'); await submit()
    expect(writes('PUT')[0]![1]!.body).toMatchObject({ updated_at: 'T1', kind: 'income' })
  })

  it('invoice create body carries no updated_at', async () => {
    transport().mockResolvedValue({ data: makeInvoice({ status: 'draft' }) })
    await useInvoiceStore().createInvoice({ gig_id: '00000000-0000-4000-8000-000000000001' })
    expect('updated_at' in writes('POST')[0]![1]!.body!).toBe(false)
  })
})

describe('FE-1a edit id missing from the cache', () => {
  it('reloads the entry, adopts its kind + token, then saves with them', async () => {
    const s = useEarningsStore(); s.openCreate('income'); s.setCreateOpen(false)
    const get = deferred<unknown>(); transport().mockReturnValueOnce(get.promise)
    s.openEdit('gone')
    w = mount(EarningsWorkspace, { attachTo: document.body }); await flushPromises()
    expect((document.querySelector('button[type=submit]') as HTMLButtonElement).disabled).toBe(true)
    get.resolve({ data: row({ id: 'gone', kind: 'expense', category: 'gear', description: '', notes: 'Cable', updated_at: 'T9' }) }); await flushPromises()
    expect(document.body.textContent).toContain('EDIT EXPENSE')
    transport().mockResolvedValue({ data: row({ id: 'gone', kind: 'expense' }) })
    await type('#ee-amount', '7'); await submit()
    expect(writes('PUT')[0]![1]!.body).toMatchObject({ kind: 'expense', updated_at: 'T9' })
  })

  it('refuses to save when the reload fails: no PUT, no empty token', async () => {
    const s = useEarningsStore(); s.openEdit('gone')
    transport().mockRejectedValue(new FinanceApiError(404, 'not_found', 'Gone'))
    w = mount(EarningsWorkspace, { attachTo: document.body }); await flushPromises()
    await type('#ee-amount', '7'); await type('#ee-description', 'Typed anyway')
    expect((document.querySelector('button[type=submit]') as HTMLButtonElement | null)?.disabled ?? true).toBe(true)
    await submit()
    expect(writes('PUT')).toHaveLength(0)
  })
})

describe('FE-5 confirmed invoice write after selection moved on', () => {
  const setup = () => {
    const s = useInvoiceStore()
    const a = makeInvoice({ id: 'a', status: 'draft', updated_at: 'T1' })
    s.invoices = [a, makeInvoice({ id: 'b', status: 'draft' })]; s.current = a; s.detailId = 'a'; s.detailOpen = true
    return { s, a }
  }
  it.each(['update', 'cancel', 'credit', 'correct'] as const)('%s updates the list row but not current', async (op) => {
    const { s, a } = setup()
    const write = deferred<unknown>(); const next = deferred<unknown>()
    transport().mockReturnValueOnce(write.promise).mockReturnValueOnce(next.promise).mockResolvedValue({ data: [] })
    const done = op === 'update' ? s.updateInvoice('a', { customer: a.customer, vat_treatment: 'domestic', tax_rate_bps: 1900, tax_note: '', withholding_rate_bps: 0, supply_date: null, due_at: null, number_prefix: 'INV', internal_notes: 'changed', updated_at: 'T1' })
      : op === 'cancel' ? s.cancelInvoice('a', 'T1')
        : op === 'credit' ? s.issueCreditNote('a', 'r', 'T1') : s.correctInvoice('a', 'r', 'T1')
    const selection = s.openDetail('b')
    const original = { ...a, status: op === 'update' ? 'draft' : op === 'cancel' ? 'cancelled' : 'credited', internal_notes: 'changed', updated_at: 'T2' } as typeof a
    const credit = makeInvoice({ id: 'cn', kind: 'credit_note', status: 'issued' }); const repl = makeInvoice({ id: 'rp', status: 'draft' })
    write.resolve({ data: op === 'credit' ? { credit_note: credit, original } : op === 'correct' ? { credit_note: credit, original, replacement: repl } : original })
    await done; await flushPromises()
    const rowA = s.invoices.find(i => i.id === 'a')!
    expect(rowA.updated_at).toBe('T2'); expect(rowA.status).toBe(original.status)
    if (op === 'credit' || op === 'correct') expect(s.invoices.some(i => i.id === 'cn')).toBe(true)
    if (op === 'correct') expect(s.invoices.some(i => i.id === 'rp')).toBe(true)
    expect(s.current).toBeNull(); expect(s.currentLoading).toBe(true)
    next.resolve({ data: makeInvoice({ id: 'b', status: 'draft' }) }); await selection
    expect(s.current?.id).toBe('b')
  })
})

describe('entry reads and deletes', () => {
  it('a list GET started after a confirmed delete may return the row (authoritative, not hidden forever)', async () => {
    const s = useEarningsStore(); s.entries = [row()]
    transport().mockResolvedValueOnce({})
    await s.deleteEntry('e', { updated_at: 'T1' })
    expect(s.entries).toEqual([])
    transport().mockResolvedValueOnce({ data: [row()] })
    await s.fetchEntries()
    expect(s.entries.map(e => e.id)).toEqual(['e'])
  })
  it('a per-entry GET started after a confirmed delete is installed; one started before is not', async () => {
    const s = useEarningsStore(); s.entries = [row()]
    const early = deferred<unknown>(); transport().mockReturnValueOnce(early.promise)
    const pre = s.fetchEntry('e')
    transport().mockResolvedValueOnce({})
    await s.deleteEntry('e', { updated_at: 'T1' })
    early.resolve({ data: row() }); await pre
    expect(s.entries).toEqual([])
    transport().mockResolvedValueOnce({ data: row({ updated_at: 'T3' }) })
    await s.fetchEntry('e')
    expect(s.entries.map(e => e.updated_at)).toEqual(['T3'])
  })
  it('delete 409 keeps the row and dialog, shows the error, and retry resends the same captured token', async () => {
    transport().mockImplementation(async (u) => ({ data: String(u).endsWith('/entries') ? [row()] : {} }))
    w = mount(Finance, { attachTo: document.body, global: { stubs: { InvoiceList: true, InvoiceWorkspace: true, EntryReconciliationPanel: true, EntryReconciliationDialog: true } } }); await flushPromises()
    const s = useEarningsStore()
    w.findComponent({ name: 'EntryList' }).vm.$emit('delete', s.entries[0]); await flushPromises()
    const del = () => [...document.querySelectorAll<HTMLButtonElement>('.finance-confirm button')].find(b => b.textContent?.trim() === 'DELETE')!
    transport().mockImplementation(async (u, o) => {
      if (o?.method === 'DELETE') throw new FinanceApiError(409, 'conflict', 'Changed elsewhere')
      if (String(u).endsWith('/entries/e')) return { data: row({ updated_at: 'T2' }) }
      return { data: {} }
    })
    del().click(); await flushPromises()
    expect(s.entries.map(e => e.id)).toEqual(['e'])
    // The 409 refreshes the cached row, so the next confirmation opened from
    // the list captures the server's new token instead of the stale one.
    expect(s.entries.map(e => e.updated_at)).toEqual(['T2'])
    expect(document.querySelector('.finance-confirm')).not.toBeNull()
    expect(document.body.textContent).toContain('Changed elsewhere')
    expect(writes('DELETE')).toHaveLength(1)
    s.entries = [row({ updated_at: 'T2' })] // refreshed cache must not leak into the retry
    del().click(); await flushPromises()
    expect(writes('DELETE').map(c => c[1]!.body)).toEqual([{ updated_at: 'T1' }, { updated_at: 'T1' }])
  })
})

const slots = defineComponent({ setup(_, { slots }) { return () => h('div', slots.default?.()) } })
function sheet() {
  return mount(InvoiceDetailSheet, { global: { stubs: { Sheet: slots, DialogTitle: slots, DialogDescription: slots, InvoiceDraftEditor: true, InvoiceTotals: true, InvoicePartySummary: true, InvoiceBalance: true, PaymentLedger: true, InvoiceActionsMenu: true, InvoiceConfirmDialog: true, NuxtLink: slots } } })
}
async function startAction(wrapper: VueWrapper, reason = 'Reason'): Promise<void> {
  wrapper.findComponent({ name: 'InvoiceActionsMenu' }).vm.$emit('action', 'credit')
  await vi.runAllTimersAsync(); await flushPromises()
  wrapper.findComponent({ name: 'InvoiceConfirmDialog' }).vm.$emit('confirm', reason); await flushPromises()
}
describe('InvoiceDetailSheet lifetime', () => {
  const open = (id = 'a') => { const s = useInvoiceStore(); const inv = makeInvoice({ id, updated_at: 'T1' }); s.gigsLoaded = true; s.detailOpen = true; s.detailId = id; s.current = inv; s.invoices = [inv]; return s }
  const res = (id = 'a') => ({ data: { credit_note: makeInvoice({ id: 'cn', kind: 'credit_note' }), original: makeInvoice({ id, status: 'credited', updated_at: 'T2' }) } })

  it.each(['cancel', 'switch', 'same-id', 'unmount'] as const)('old completion after %s: no toast, no error, new busy state intact', async (mode) => {
    vi.useFakeTimers()
    const s = open(); w = sheet()
    const old = deferred<unknown>(); transport().mockReturnValueOnce(old.promise)
    await startAction(w)
    expect(w.findComponent({ name: 'InvoiceConfirmDialog' }).props('busy')).toBe(true)
    let fresh: ReturnType<typeof deferred<unknown>> | undefined
    if (mode === 'unmount') w.unmount()
    else if (mode === 'cancel') { s.setDetailOpen(false); await flushPromises() }
    else {
      s.setDetailOpen(false); await flushPromises()
      const inv = makeInvoice({ id: mode === 'switch' ? 'b' : 'a', updated_at: 'T1' })
      s.detailOpen = true; s.detailId = inv.id; s.current = inv; s.invoices = [inv]; await flushPromises()
      fresh = deferred<unknown>(); transport().mockReturnValueOnce(fresh.promise)
      await startAction(w)
      expect(w.findComponent({ name: 'InvoiceConfirmDialog' }).props('busy')).toBe(true)
    }
    old.resolve(res()); await flushPromises()
    expect(toastSpy).not.toHaveBeenCalled()
    if (mode !== 'unmount') {
      expect(w.find('.ids-error').exists()).toBe(false)
      if (fresh) expect(w.findComponent({ name: 'InvoiceConfirmDialog' }).props('busy')).toBe(true)
      else expect(w.findComponent({ name: 'InvoiceConfirmDialog' }).props('busy')).toBe(false)
    }
    if (fresh) { fresh.resolve(res(mode === 'switch' ? 'b' : 'a')); await flushPromises() }
  })

  it.each(['cancel', 'same-id', 'unmount'] as const)('old rejection after %s shows no error', async (mode) => {
    vi.useFakeTimers()
    const s = open(); w = sheet()
    const old = deferred<unknown>(); transport().mockReturnValueOnce(old.promise)
    await startAction(w)
    if (mode === 'unmount') w.unmount()
    else { s.setDetailOpen(false); await flushPromises(); if (mode === 'same-id') { s.detailOpen = true; await flushPromises() } }
    old.reject(new FinanceApiError(400, 'validation_failed', 'Old bad reason')); await flushPromises()
    if (mode !== 'unmount') { expect(w.find('.ids-error').exists()).toBe(false); expect(w.findComponent({ name: 'InvoiceConfirmDialog' }).props('error')).toBe('') }
    expect(toastSpy).not.toHaveBeenCalled()
  })

  it('current session completion still toasts', async () => {
    vi.useFakeTimers()
    open(); w = sheet()
    transport().mockResolvedValueOnce(res()).mockResolvedValue({ data: makeInvoice({ id: 'a', status: 'credited' }), lines: [] })
    await startAction(w); await vi.runAllTimersAsync(); await flushPromises()
    expect(toastSpy).toHaveBeenCalledTimes(1)
  })
})

describe('InvoiceDraftEditor same-ID reopen', () => {
  const draft = (over = {}) => makeInvoice({ id: 'a', status: 'draft', updated_at: 'T1', ...over })
  const mountOpen = () => {
    const s = useInvoiceStore(); s.current = draft(); s.detailOpen = true
    const Parent = defineComponent({ setup() { return () => (s.detailOpen && s.current ? h(InvoiceDraftEditor, { invoice: s.current }) : null) } })
    w = mount(Parent, { global: { stubs: { InvoicePartySummary: true, InvoicePartyFields: true, InvoiceTaxFields: true, InvoiceIssueChecklist: true } } })
    return s
  }
  it.each(['resolve', 'reject'] as const)('old save %s after close + same-ID reopen leaves the new editor untouched', async (outcome) => {
    const s = mountOpen()
    await w!.find('textarea').setValue('Old local')
    const old = deferred<unknown>(); transport().mockReturnValueOnce(old.promise).mockResolvedValue({ data: draft() })
    await w!.find('form').trigger('submit'); await flushPromises()
    s.detailOpen = false; await flushPromises()
    s.detailOpen = true; await flushPromises()
    await w!.find('textarea').setValue('New local')
    if (outcome === 'resolve') old.resolve({ data: draft({ internal_notes: 'Old local', updated_at: 'T2' }) })
    else old.reject(new FinanceApiError(409, 'conflict', 'Old conflict'))
    await flushPromises()
    expect((w!.find('textarea').element as HTMLTextAreaElement).value).toBe('New local')
    expect(w!.text()).not.toMatch(/changed elsewhere/i)
    expect(writes('PUT')).toHaveLength(1)
    expect(w!.emitted('saved')).toBeUndefined()
  })
})
