// Quality-review follow-ups (real Pinia, mounted components, deferred transport).
//   Important 1: the "Replacement draft ready" toast survives the store's own
//                detailId change after Correct (real confirm dialog + menu).
//   Important 2: conflict copy tells the user to discard and reload (Save is
//                disabled while conflicted, so "keep edits and retry" lied).
//   Minors: reload failure message, a11y focus/alert on conflict, superseded
//           reads are not errors, props.kind change keeps unsaved edits.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import EntryFormDialog from '../EntryFormDialog.vue'
import InvoiceDetailSheet from '../InvoiceDetailSheet.vue'
import InvoiceDraftEditor from '../InvoiceDraftEditor.vue'
import { useEarningsStore } from '../../../stores/earnings'
import { FinanceApiError, useInvoiceStore } from '../../../stores/invoice'
import { makeInvoice } from './fixtures'
import type { Entry } from '../../../types/finance'

const toastSpy = vi.hoisted(() => vi.fn())
vi.mock('#kui/components/ui/toast/use-toast', () => ({ useToast: () => ({ toast: toastSpy }) }))

type Call = [string, { method?: string; body?: Record<string, unknown> }?]
const transport = () => vi.mocked($fetch as unknown as (url: string, options?: Call[1]) => Promise<unknown>)
const entry = (over: Partial<Entry> = {}): Entry => ({ id: 'e', kind: 'income', amount_minor: 100, currency: 'EUR', category: 'gig_fee', entry_date: '2026-09-01', description: 'Fee', notes: '', gig_id: null, status: 'active', auto_generated: false, source_kind: 'manual', source_id: null, source_amount_minor: null, source_currency: null, source_description: '', created_at: 'T1', updated_at: 'T1', deleted_at: null, ...over })
function deferred<T>() { let resolve!: (v: T) => void; let reject!: (e: unknown) => void; const promise = new Promise<T>((a, b) => { resolve = a; reject = b }); return { promise, resolve, reject } }
const slots = defineComponent({ setup(_, { slots }) { return () => h('div', slots.default?.()) } })
const CONFLICT_COPY = 'Discard your changes and reload to continue.'

let w: VueWrapper | undefined
beforeEach(() => { setActivePinia(createPinia()); vi.stubGlobal('useHead', vi.fn()); vi.stubGlobal('$fetch', vi.fn()); document.body.replaceChildren(); toastSpy.mockClear() })
afterEach(() => { w?.unmount(); w = undefined; vi.unstubAllGlobals(); vi.restoreAllMocks(); vi.useRealTimers() })

describe('Important 1: Correct toast', () => {
  it('toasts "Replacement draft ready" after a confirmed Correct even though the store moves detailId', async () => {
    vi.useFakeTimers()
    const s = useInvoiceStore()
    const inv = makeInvoice({ id: 'a', status: 'issued', kind: 'invoice', updated_at: 'T1' })
    s.gigsLoaded = true; s.detailOpen = true; s.detailId = 'a'; s.current = inv; s.invoices = [inv]
    const repl = makeInvoice({ id: 'r', status: 'draft', updated_at: 'T1' })
    transport().mockImplementation(async (url, o) => {
      if (o?.method === 'POST' && String(url).endsWith('/correct')) {
        return { data: { credit_note: makeInvoice({ id: 'cn', kind: 'credit_note', credits_invoice_id: 'a' }), original: makeInvoice({ id: 'a', status: 'corrected', replaced_by_invoice_id: 'r' }), replacement: repl } }
      }
      if (String(url).endsWith('/invoices/r')) return { data: repl, lines: [] }
      return { data: { ready: true, problems: [] } }
    })
    // Only the Sheet shell and leaf children are stubbed: the real actions
    // menu and the real confirm dialog drive the flow.
    w = mount(InvoiceDetailSheet, { attachTo: document.body, global: { stubs: { Sheet: slots, DialogTitle: slots, DialogDescription: slots, InvoiceDraftEditor: true, InvoiceTotals: true, InvoicePartySummary: true, InvoiceBalance: true, PaymentLedger: true, NuxtLink: slots } } })
    await flushPromises()
    expect(w.findComponent({ name: 'InvoiceActionsMenu' }).exists()).toBe(true)
    w.findComponent({ name: 'InvoiceActionsMenu' }).vm.$emit('action', 'correct')
    await vi.runAllTimersAsync(); await flushPromises()
    const ta = document.body.querySelector('textarea') as HTMLTextAreaElement
    expect(ta).not.toBeNull()
    ta.value = 'Wrong VAT'; ta.dispatchEvent(new Event('input', { bubbles: true })); await flushPromises()
    const confirm = Array.from(document.body.querySelectorAll('button')).find((b) => /CREDIT & CREATE DRAFT/.test(b.textContent ?? ''))!
    confirm.click()
    await vi.runAllTimersAsync(); await flushPromises()
    expect(s.detailId).toBe('r')
    expect(toastSpy).toHaveBeenCalledTimes(1)
    expect(toastSpy.mock.calls[0]![0]).toMatchObject({ title: 'Replacement draft ready' })
  })
})

describe('Important 2: conflict copy', () => {
  it('entry dialog says discard and reload; no retry promise; reload button gets focus', async () => {
    const s = useEarningsStore(); s.entries = [entry()]
    w = mount(EntryFormDialog, { attachTo: document.body, props: { open: true, kind: 'income', editId: 'e' } }); await flushPromises()
    // useDialogFocus moves focus to the title on a real ~16ms timer. Let that
    // settle first, otherwise on a slow runner it fires after the conflict
    // handler focuses the reload button and steals focus back.
    await vi.waitFor(() => expect(document.activeElement).toBe(document.querySelector('.ee-title')))
    transport().mockRejectedValueOnce(new FinanceApiError(409, 'conflict', 'Changed')).mockResolvedValueOnce({ data: entry({ updated_at: 'T2' }) })
    ;(document.querySelector('#ee-description') as HTMLInputElement).value = 'Mine'
    document.querySelector('#ee-description')!.dispatchEvent(new Event('input', { bubbles: true }))
    document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); await flushPromises()
    const alert = document.querySelector('[role="alert"]')!
    expect(alert.textContent).toContain(CONFLICT_COPY)
    expect(alert.textContent).not.toMatch(/keep your edits|retry/i)
    expect(document.activeElement).toBe(document.querySelector('[data-testid="ee-discard-reload"]'))
  })

  it('invoice editor says discard and reload in an alert; reload button gets focus', async () => {
    const s = useInvoiceStore(); const inv = makeInvoice({ id: 'a', status: 'draft', updated_at: 'T1' }); s.current = inv; s.invoices = [inv]
    w = mount(InvoiceDraftEditor, { attachTo: document.body, props: { invoice: inv }, global: { stubs: { InvoicePartyFields: true, InvoiceTaxFields: true, InvoiceIssueChecklist: true } } })
    await w.find('textarea').setValue('Mine')
    transport().mockRejectedValueOnce(new FinanceApiError(409, 'conflict', 'Changed')).mockResolvedValue({ data: makeInvoice({ id: 'a', status: 'draft', updated_at: 'T2' }), lines: [] })
    await w.find('form').trigger('submit'); await flushPromises()
    const alert = w.find('[role="alert"]')
    expect(alert.exists()).toBe(true)
    expect(alert.text()).toContain(CONFLICT_COPY)
    expect(alert.text()).not.toMatch(/keep your edits|retry/i)
    expect(document.activeElement).toBe(w.find('[data-testid="inv-discard-reload"]').element)
  })
})

describe('minors', () => {
  it('invoice discardAndReload shows an error when the reload fails', async () => {
    const s = useInvoiceStore(); const inv = makeInvoice({ id: 'a', status: 'draft', updated_at: 'T1' }); s.current = inv; s.invoices = [inv]
    w = mount(InvoiceDraftEditor, { attachTo: document.body, props: { invoice: inv }, global: { stubs: { InvoicePartyFields: true, InvoiceTaxFields: true, InvoiceIssueChecklist: true } } })
    await w.find('textarea').setValue('Mine')
    transport().mockRejectedValueOnce(new FinanceApiError(409, 'conflict', 'Changed')).mockRejectedValue(new FinanceApiError(500, 'unknown', 'boom'))
    await w.find('form').trigger('submit'); await flushPromises()
    await w.find('[data-testid="inv-discard-reload"]').trigger('click'); await flushPromises()
    expect(w.find('.draft-status').text()).toContain('Could not reload the invoice.')
  })

  it('a superseded entry read with nothing cached is retried, so the dialog loads instead of staying blank', async () => {
    const s = useEarningsStore()
    const first = deferred<unknown>()
    transport().mockReturnValueOnce(first.promise)
    w = mount(EntryFormDialog, { attachTo: document.body, props: { open: true, kind: 'income', editId: 'e' } }); await flushPromises()
    const second = deferred<unknown>(); transport().mockReturnValueOnce(second.promise)
    const retry = deferred<unknown>(); transport().mockReturnValueOnce(retry.promise)
    const newer = s.fetchEntry('e')
    first.resolve({ data: entry() }); await flushPromises()
    // Superseded and nothing cached: no error, and the dialog asks again.
    expect(document.querySelector('[role="alert"]')).toBeNull()
    expect(transport()).toHaveBeenCalledTimes(3)
    retry.resolve({ data: entry({ description: 'Loaded on retry' }) }); await flushPromises()
    expect((document.querySelector('#ee-description') as HTMLInputElement).value).toBe('Loaded on retry')
    expect((document.querySelector('button[type="submit"]') as HTMLButtonElement).disabled).toBe(false)
    expect(document.querySelector('[role="alert"]')).toBeNull()
    second.resolve({ data: entry() }); await newer; await flushPromises()
  })

  it('a superseded entry read that stays superseded with nothing cached ends in a real error, not a blank dialog', async () => {
    const s = useEarningsStore()
    const first = deferred<unknown>(); transport().mockReturnValueOnce(first.promise)
    w = mount(EntryFormDialog, { attachTo: document.body, props: { open: true, kind: 'income', editId: 'e' } }); await flushPromises()
    const second = deferred<unknown>(); transport().mockReturnValueOnce(second.promise)
    const retry = deferred<unknown>(); transport().mockReturnValueOnce(retry.promise)
    const newer = s.fetchEntry('e')
    first.resolve({ data: entry() }); await flushPromises()
    const third = deferred<unknown>(); transport().mockReturnValueOnce(third.promise)
    const newest = s.fetchEntry('e') // supersedes the retry as well
    retry.resolve({ data: entry() }); await flushPromises()
    expect(document.querySelector('[role="alert"]')?.textContent).toContain('Could not load this entry')
    second.resolve({ data: entry() }); third.resolve({ data: entry() }); await newer; await newest; await flushPromises()
  })

  it('a superseded reload read during discard is not reported as an error', async () => {
    const s = useEarningsStore(); s.entries = [entry()]
    w = mount(EntryFormDialog, { attachTo: document.body, props: { open: true, kind: 'income', editId: 'e' } }); await flushPromises()
    transport().mockRejectedValueOnce(new FinanceApiError(409, 'conflict', 'Changed')).mockResolvedValueOnce({ data: entry({ updated_at: 'T2' }) })
    document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); await flushPromises()
    const first = deferred<unknown>(); transport().mockReturnValueOnce(first.promise)
    ;(document.querySelector('[data-testid="ee-discard-reload"]') as HTMLButtonElement).click(); await flushPromises()
    const second = deferred<unknown>(); transport().mockReturnValueOnce(second.promise)
    const newer = s.fetchEntry('e')
    first.resolve({ data: entry({ updated_at: 'T2' }) }); await flushPromises()
    expect(document.body.textContent).not.toMatch(/Could not reload the entry/)
    second.resolve({ data: entry({ updated_at: 'T2' }) }); await newer; await flushPromises()
  })

  it('a props.kind change while editing keeps unsaved edits', async () => {
    const s = useEarningsStore(); s.entries = [entry()]
    w = mount(EntryFormDialog, { attachTo: document.body, props: { open: true, kind: 'income', editId: 'e' } }); await flushPromises()
    const el = document.querySelector('#ee-description') as HTMLInputElement
    el.value = 'Unsaved'; el.dispatchEvent(new Event('input', { bubbles: true })); await flushPromises()
    await w.setProps({ kind: 'expense' }); await flushPromises()
    expect((document.querySelector('#ee-description') as HTMLInputElement).value).toBe('Unsaved')
  })
})
