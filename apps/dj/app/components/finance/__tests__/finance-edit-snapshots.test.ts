// FE-1 / FE-2 finance hardening — regression tests asserting desired
// editor behavior. These are RED-GREEN tests that fail against the buggy
// base and pass against the fixed code:
//
//   FE-1: Editing an entry must use the entry's own kind, not the last
//         create kind. Editing expenses preserves Notes (the expense branch)
//         and editing income preserves Description. Mounted real-Pinia
//         workspace/dialog coverage with deferred transport stubs.
//
//   FE-2: Editor fields and the original updated_at are one snapshot.
//         Same-ID store refresh preserves the unsaved local fields and
//         their original T1 token rather than adopting the refreshed T2
//         token. On 409 the editor preserves the unsaved form, blocks blind
//         Save, and only the explicit DISCARD AND RELOAD button adopts the
//         authoritative fields and version together.
//
// Both directions covered (income + expense for FE-1, entry + invoice for
// FE-2). The deferred transport stubs keep the tests mounted against the
// real Pinia store + dialog logic.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import EarningsWorkspace from '../EarningsWorkspace.vue'
import InvoiceDraftEditor from '../InvoiceDraftEditor.vue'
import { FinanceApiError, useInvoiceStore } from '../../../stores/invoice'
import { useEarningsStore } from '../../../stores/earnings'
import { makeInvoice } from './fixtures'
import type { Entry, Invoice } from '../../../types/finance'

// Narrow transport typing avoids instantiating Nitro's recursive route overloads in mocks.
const transport = () => vi.mocked($fetch as unknown as (url: string, options?: { method?: string; body?: unknown }) => Promise<unknown>)
function entry(over: Partial<Entry> = {}): Entry {
  return {
    id: 'e-1', kind: 'income', amount_minor: 10000, currency: 'EUR', category: 'gig_fee',
    entry_date: '2026-09-15', description: 'Old description', notes: '', gig_id: null,
    status: 'active', auto_generated: false, source_kind: 'manual', source_id: null,
    source_amount_minor: null, source_currency: null, source_description: '',
    created_at: '2026-09-15T10:00:00Z', updated_at: '2026-09-15T10:00:00Z',
    deleted_at: null, ...over,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

let wrappers: VueWrapper[] = []
async function field(selector: string, value: string): Promise<void> {
  const el = document.querySelector(selector) as HTMLInputElement | HTMLTextAreaElement
  expect(el).not.toBeNull()
  el.value = value
  el.dispatchEvent(new Event('input', { bubbles: true }))
  el.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}
async function submit(): Promise<void> {
  const form = document.querySelector('form.ee-form') as HTMLFormElement | null
  expect(form).not.toBeNull()
  form!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await flushPromises()
}

describe('FE-1 entry editor preserves the entry kind', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    // Test setup: clear jsdom between tests so each run gets a fresh dialog mount.
    document.body.innerHTML = ''
    vi.stubGlobal('$fetch', vi.fn())
  })
  afterEach(() => {
    wrappers.forEach((w) => w.unmount())
    wrappers = []
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it.each([
    { createKind: 'income' as const, editKind: 'expense' as const },
    { createKind: 'expense' as const, editKind: 'income' as const },
    { createKind: 'income' as const, editKind: 'income' as const },
    { createKind: 'expense' as const, editKind: 'expense' as const },
  ])(
    'after create $createKind, edit preserves $editKind identity, readback matches',
    async ({ createKind, editKind }) => {
      const store = useEarningsStore()
      const original = entry(editKind === 'expense'
        ? { kind: 'expense', category: 'gear', description: '', notes: 'Cable purchase' }
        : { kind: 'income', category: 'gig_fee', description: 'Old description', notes: '' })
      store.entries = [original]
      store.openCreate(createKind)
      store.setCreateOpen(false)
      store.openEdit(original.id)
      wrappers.push(mount(EarningsWorkspace, { attachTo: document.body }))
      await flushPromises()

      // Title reflects the entry's kind, never the last create kind.
      expect(document.body.textContent).toContain(`EDIT ${editKind.toUpperCase()}`)
      if (editKind !== createKind) {
        expect(document.body.textContent).not.toContain(`EDIT ${createKind.toUpperCase()}`)
      }

      // The required-string field is the one appropriate to the entry's kind.
      if (editKind === 'expense') {
        expect(document.querySelector('#ee-notes')).not.toBeNull()
        expect(document.querySelector('#ee-description')).toBeNull()
        expect((document.querySelector('#ee-notes') as HTMLTextAreaElement).value).toBe('Cable purchase')
      } else {
        expect(document.querySelector('#ee-description')).not.toBeNull()
        expect(document.querySelector('#ee-notes')).toBeNull()
        expect((document.querySelector('#ee-description') as HTMLInputElement).value).toBe('Old description')
      }

      // Mutate the amount so we can verify the readback is the entry's own kind.
      await field('#ee-amount', '110')
      let persisted = { ...original }
      transport().mockImplementation(async (_url, opts) => {
        if (opts?.method === 'PUT') persisted = { ...persisted, ...(opts.body as object) }
        return { data: persisted }
      })
      await submit()

      // The PUT body must carry the entry's own kind + correct branch field, not
      // the last create kind.
      const putCall = transport().mock.calls.find(([, opts]) => opts?.method === 'PUT')
      expect(putCall?.[1]).toMatchObject({
        method: 'PUT',
        body: {
          kind: editKind,
          amount_minor: 11000,
          notes: original.notes,
          description: original.description,
        },
      })

      // Readback from the store must reflect the edited entry with the entry's own kind.
      const readback = await store.fetchEntry(original.id)
      expect(readback).toMatchObject({
        kind: editKind,
        amount_minor: 11000,
        notes: original.notes,
        description: original.description,
      })
    },
  )
})

describe('FE-2 entry editor preserves local fields and the original T1 token', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    // Test setup: clear jsdom between tests so each run gets a fresh dialog mount.
    document.body.innerHTML = ''
    vi.stubGlobal('$fetch', vi.fn())
  })
  afterEach(() => {
    wrappers.forEach((w) => w.unmount())
    wrappers = []
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('deferred same-ID list response (T1->T2) retains local fields and submits with the original T1 token', async () => {
    const store = useEarningsStore()
    const original = entry({ amount_minor: 10000, description: 'Old description', updated_at: '2026-09-15T10:00:00Z' })
    store.entries = [original]
    store.openEdit(original.id)
    wrappers.push(mount(EarningsWorkspace, { attachTo: document.body }))
    await flushPromises()

    // User edits the description locally — the editor's snapshot is now
    // T1-fields + T1-token, completely independent of the cache.
    await field('#ee-description', 'Local draft')

    // A deferred same-ID list refresh resolves later with T2 (different amount
    // + a fresh updated_at). The editor must NOT silently adopt T2 fields
    // or T2 token.
    const refresh = deferred<{ data: Entry[] }>()
    transport().mockReturnValueOnce(refresh.promise)
    const pending = store.fetchEntries()
    await flushPromises()

    // The list is still pending — the local fields must remain exactly what
    // the user typed, and the token must still be T1.
    expect((document.querySelector('#ee-description') as HTMLInputElement).value).toBe('Local draft')
    expect((document.querySelector('#ee-amount') as HTMLInputElement).value).toBe('100.00')

    refresh.resolve({
      data: [entry({ amount_minor: 90000, description: 'Remote saved', updated_at: '2026-09-15T11:00:00Z' })],
    })
    await pending
    await flushPromises()

    // Even after T2 lands, the editor keeps its T1 fields and T1 token.
    expect((document.querySelector('#ee-description') as HTMLInputElement).value).toBe('Local draft')
    expect((document.querySelector('#ee-amount') as HTMLInputElement).value).toBe('100.00')

    // Submitting the editor must POST with T1 fields + T1 token, not the
    // refreshed T2 fields / T2 updated_at.
    transport().mockResolvedValueOnce({ data: entry({ description: 'Local draft' }) })
    await submit()
    const writes = transport().mock.calls.filter(([, opts]) => opts?.method === 'PUT')
    expect(writes).toHaveLength(1)
    expect(writes[0]?.[1]).toMatchObject({
      method: 'PUT',
      body: {
        amount_minor: 10000,
        description: 'Local draft',
        updated_at: original.updated_at,
      },
    })
  })

  it('409 conflict refetch keeps the unsaved draft; blind retry is blocked until DISCARD AND RELOAD adopts a complete snapshot', async () => {
    const store = useEarningsStore()
    const original = entry({ amount_minor: 10000, description: 'Old description', updated_at: '2026-09-15T10:00:00Z' })
    store.entries = [original]
    store.openEdit(original.id)
    wrappers.push(mount(EarningsWorkspace, { attachTo: document.body }))
    await flushPromises()

    // User edits the description — unsaved T1 draft.
    await field('#ee-description', 'Local draft')

    // First PUT is rejected with 409. The store then performs an authoritative
    // GET for the entry. The PUT must carry T1 fields + T1 token (the dialog's
    // own snapshot), not T2 (which the store has not seen yet).
    const remote = entry({ amount_minor: 90000, description: 'Remote saved', updated_at: '2026-09-15T11:00:00Z' })
    const authoritative = deferred<{ data: Entry }>()
    transport()
      .mockRejectedValueOnce(new FinanceApiError(409, 'conflict', 'Changed'))
      .mockImplementationOnce(async (url) => {
        const result = await authoritative.promise
        return String(url).endsWith(`/${original.id}`) ? result : { data: [result.data] }
      })
    await submit()

    // The PUT must carry T1 fields + T1 token, NOT T2 fields / T2 token.
    const firstPut = transport().mock.calls.find(([, opts]) => opts?.method === 'PUT')
    expect(firstPut?.[1]).toMatchObject({
      method: 'PUT',
      body: {
        amount_minor: 10000,
        description: 'Local draft',
        updated_at: original.updated_at,
      },
    })

    // The local draft must still be in the form, untouched by the rejection.
    expect((document.querySelector('#ee-description') as HTMLInputElement).value).toBe('Local draft')

    // The store performs an authoritative GET for the same entry id; the
    // dialog keeps its unsaved fields during that round-trip.
    expect(transport().mock.calls[1]?.[0]).toBe(`/api/v1/finance/entries/${original.id}`)

    authoritative.resolve({ data: remote })
    await flushPromises()

    // Even after the authoritative GET installs T2 fields, the editor must
    // keep its unsaved T1 fields and T1 token. The Save button must be
    // disabled until the user explicitly opts in via DISCARD AND RELOAD.
    expect((document.querySelector('#ee-description') as HTMLInputElement).value).toBe('Local draft')
    const saveBtn = document.querySelector('button[type="submit"]') as HTMLButtonElement
    expect(saveBtn.disabled).toBe(true)

    // Blind repeat Save attempts must NOT go through while conflict state is
    // unresolved. The user has not yet adopted the authoritative snapshot.
    await submit()
    const writesAfterRetry = transport().mock.calls.filter(([, opts]) => opts?.method === 'PUT')
    expect(writesAfterRetry).toHaveLength(1)

    // The conflict banner + explicit DISCARD CHANGES AND RELOAD button are
    // surfaced.
    const reloadBtn = Array.from(document.querySelectorAll('button')).find((b) =>
      /DISCARD.*RELOAD/.test(b.textContent ?? ''))
    expect(reloadBtn).toBeDefined()

    // Clicking DISCARD AND RELOAD performs the same authoritative GET; the
    // dialog must adopt both the new fields AND the new version token
    // together.
    transport().mockResolvedValueOnce({ data: remote })
    ;(reloadBtn as HTMLButtonElement).click()
    await flushPromises()
    expect((document.querySelector('#ee-description') as HTMLInputElement).value).toBe('Remote saved')
    expect((document.querySelector('#ee-amount') as HTMLInputElement).value).toBe('900.00')

    // Now Save is enabled again, and any further edit uses the fresh T2
    // snapshot. Editing description again should produce a PUT body with
    // the adopted fields + T2 token.
    await field('#ee-description', 'Reviewed change')
    transport().mockResolvedValueOnce({ data: { ...remote, description: 'Reviewed change' } })
    await submit()
    const writes = transport().mock.calls.filter(([, opts]) => opts?.method === 'PUT')
    expect(writes).toHaveLength(2)
    expect(writes[1]?.[1]).toMatchObject({
      method: 'PUT',
      body: {
        amount_minor: 90000,
        description: 'Reviewed change',
        updated_at: remote.updated_at,
      },
    })
  })
})

describe('FE-2 invoice draft editor preserves local fields and the original T1 token', () => {
  function invoice(over: Partial<Invoice> = {}): Invoice {
    return makeInvoice({
      status: 'draft',
      customer: { ...makeInvoice().customer, country: 'DE' },
      internal_notes: 'Original note',
      updated_at: '2026-09-15T10:00:00Z',
      ...over,
    })
  }

  let wrapper: VueWrapper | undefined
  function editor(): VueWrapper {
    const store = useInvoiceStore()
    const Parent = defineComponent({
      setup() { return () => store.current ? h(InvoiceDraftEditor, { invoice: store.current }) : null },
    })
    wrapper = mount(Parent, {
      global: { stubs: { InvoicePartyFields: true, InvoiceTaxFields: true, InvoiceIssueChecklist: true } },
    })
    return wrapper
  }

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.stubGlobal('$fetch', vi.fn())
  })
  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    wrappers.forEach((w) => w.unmount())
    wrappers = []
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('deferred same-ID invoice refresh preserves local fields and submits with the original T1 token', async () => {
    const store = useInvoiceStore()
    const original = invoice()
    store.current = original
    store.invoices = [original]
    const w = editor()
    await w.find('textarea').setValue('Local note')

    // Deferred authoritative GET returns T2 with different tax.
    const refresh = deferred<{ data: Invoice; lines: [] }>()
    transport().mockReturnValueOnce(refresh.promise).mockResolvedValueOnce({ data: { ready: true, problems: [] } })
    const pending = store.fetchInvoice(original.id)
    await flushPromises()
    refresh.resolve({
      data: invoice({ tax_rate_bps: 700, internal_notes: 'Remote note', updated_at: '2026-09-15T11:00:00Z' }),
      lines: [],
    })
    await pending
    await flushPromises()

    // Editor keeps its T1 field after the refresh resolves.
    expect((w.find('textarea').element as HTMLTextAreaElement).value).toBe('Local note')

    // Submitting the editor must POST with T1 fields + T1 token, not T2.
    transport().mockResolvedValueOnce({ data: original }).mockResolvedValueOnce({ data: { ready: true, problems: [] } })
    await w.find('form').trigger('submit')
    await flushPromises()
    const writes = transport().mock.calls.filter(([, opts]) => opts?.method === 'PUT')
    expect(writes).toHaveLength(1)
    expect(writes[0]?.[1]).toMatchObject({
      method: 'PUT',
      body: {
        internal_notes: 'Local note',
        tax_rate_bps: original.tax_rate_bps,
        updated_at: original.updated_at,
      },
    })
  })

  it('409 conflict authoritative GET keeps the unsaved draft; blind retry is blocked until explicit reload adopts a complete snapshot', async () => {
    const store = useInvoiceStore()
    const original = invoice()
    store.current = original
    store.invoices = [original]
    const w = editor()
    await w.find('textarea').setValue('My local note')

    const remote = invoice({ tax_rate_bps: 700, internal_notes: 'Remote note', updated_at: '2026-09-15T11:00:00Z' })
    const authoritative = deferred<{ data: Invoice; lines: [] }>()
    transport()
      .mockRejectedValueOnce(new FinanceApiError(409, 'conflict', 'Changed'))
      .mockReturnValueOnce(authoritative.promise)
      .mockResolvedValueOnce({ data: { ready: true, problems: [] } })
    await w.find('form').trigger('submit')
    await flushPromises()

    // The PUT must carry T1 fields + T1 token, not T2 (which has not been seen yet).
    const firstPut = transport().mock.calls.find(([, opts]) => opts?.method === 'PUT')
    expect(firstPut?.[1]).toMatchObject({
      method: 'PUT',
      body: {
        internal_notes: 'My local note',
        tax_rate_bps: original.tax_rate_bps,
        updated_at: original.updated_at,
      },
    })

    // The local draft remains in the textarea.
    expect((w.find('textarea').element as HTMLTextAreaElement).value).toBe('My local note')

    authoritative.resolve({ data: remote, lines: [] })
    await flushPromises()

    // After authoritative GET installs T2, the editor keeps T1 fields + T1 token.
    expect(store.current?.tax_rate_bps).toBe(700)
    expect((w.find('textarea').element as HTMLTextAreaElement).value).toBe('My local note')
    expect((w.find('button[type="submit"]').element as HTMLButtonElement).disabled).toBe(true)

    // Blind repeat Save must not go through.
    await w.find('form').trigger('submit')
    await flushPromises()
    const writesAfterRetry = transport().mock.calls.filter(([, opts]) => opts?.method === 'PUT')
    expect(writesAfterRetry).toHaveLength(1)

    // DISCARD CHANGES AND RELOAD is exposed.
    const reload = w.findAll('button').find((b) => /DISCARD.*RELOAD/.test(b.text()))!
    expect(reload.exists()).toBe(true)

    transport().mockResolvedValueOnce({ data: remote, lines: [] }).mockResolvedValueOnce({ data: { ready: true, problems: [] } })
    await reload.trigger('click')
    await flushPromises()
    expect((w.find('textarea').element as HTMLTextAreaElement).value).toBe('Remote note')

    await w.find('textarea').setValue('Reviewed note')
    transport()
      .mockResolvedValueOnce({ data: { ...remote, internal_notes: 'Reviewed note' } })
      .mockResolvedValueOnce({ data: { ready: true, problems: [] } })
    await w.find('form').trigger('submit')
    await flushPromises()
    const writes = transport().mock.calls.filter(([, opts]) => opts?.method === 'PUT')
    expect(writes).toHaveLength(2)
    expect(writes[0]?.[1]).toMatchObject({
      method: 'PUT',
      body: { internal_notes: 'My local note', tax_rate_bps: original.tax_rate_bps, updated_at: original.updated_at },
    })
    expect(writes[1]?.[1]).toMatchObject({
      method: 'PUT',
      body: { internal_notes: 'Reviewed note', tax_rate_bps: remote.tax_rate_bps, updated_at: remote.updated_at },
    })
  })
})