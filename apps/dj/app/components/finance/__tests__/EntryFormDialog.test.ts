// EntryFormDialog — TDD coverage for FIN-01 / FIN-02 / FIN-10.
// Verifies the dialog actually saves income & expense rows, pre-loads the
// gig id, rejects invalid calendar dates, refuses empty categories, and
// surfaces the FIN-10 notice copy. The dialog mirrors the API contract:
// the date is a real calendar day (no 2026-02-30) and the required-string
// branch flips between description (income) and notes (expense).
//
// Radix portals DialogContent to document.body, which is why we look for
// elements via `document.querySelector` rather than `wrapper.get`.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import EntryFormDialog from '../EntryFormDialog.vue'
import { useEarningsStore } from '../../../stores/earnings'
import type { Entry } from '../../../types/finance'

function makeEntry(over: Partial<Entry> = {}): Entry {
  return {
    id: 'e-1',
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

/** Set an input/textarea/select value through Vue's v-model contract. */
async function setField(selector: string, value: string): Promise<void> {
  const el = document.querySelector(selector) as
    | HTMLInputElement
    | HTMLTextAreaElement
    | HTMLSelectElement
    | null
  if (!el) throw new Error(`No element found for ${selector}`)
  // Vue's v-model on `<input>` listens to `input`, on `<select>` to `change`.
  // We trigger both to match real user behaviour.
  el.value = value
  el.dispatchEvent(new Event('input', { bubbles: true }))
  el.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}

function submitForm(): void {
  const form = document.querySelector('form.ee-form') as HTMLFormElement | null
  if (!form) throw new Error('No form found')
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
}

function findSaveButton(): HTMLButtonElement {
  const btn = Array.from(document.querySelectorAll('button'))
    .find((b) => /CREATE|SAVE/.test(b.textContent ?? '')) as HTMLButtonElement | undefined
  if (!btn) throw new Error('No save button found')
  return btn
}

function msgFor(selector: string): HTMLElement {
  const input = document.querySelector(selector) as HTMLElement | null
  const msg = input?.parentElement?.querySelector('.ee-msg') as HTMLElement | null
  if (!msg) throw new Error(`No message node for ${selector}`)
  return msg
}

describe('EntryFormDialog', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    document.body.innerHTML = ''
  })

  it('saves a happy-path income entry and forwards the gigId branch', async () => {
    const store = useEarningsStore()
    const saved = makeEntry({ id: 'new', amount_minor: 150000, description: 'Sample Records — Royalties' })
    const spy = vi.spyOn(store, 'createEntry').mockResolvedValue(saved)
    const gigId = '00000000-0000-4000-8000-000000000001'
    const wrapper = mount(EntryFormDialog, {
      attachTo: document.body,
      props: { open: true, kind: 'income', gigId, editId: null },
    })
    await flushPromises()
    expect((document.querySelector('#ee-gig') as HTMLInputElement).value).toBe(gigId)
    await setField('#ee-amount', '1500')
    await setField('#ee-currency', 'EUR')
    await setField('#ee-date', '2026-09-15')
    await setField('#ee-description', 'Sample Records — Royalties')
    submitForm()
    await flushPromises()
    expect(spy).toHaveBeenCalledTimes(1)
    const body = spy.mock.calls[0]![0]
    expect(body).toMatchObject({
      kind: 'income', amount_minor: 150000, currency: 'EUR', category: 'gig_fee',
      entry_date: '2026-09-15', description: 'Sample Records — Royalties', gig_id: gigId,
    })
    expect(wrapper.emitted('update:open')?.[0]).toEqual([false])
  })

  it('saves a happy-path expense entry using the notes field', async () => {
    const store = useEarningsStore()
    const saved = makeEntry({ id: 'new-e', kind: 'expense', amount_minor: 5000, notes: 'New cable' })
    const spy = vi.spyOn(store, 'createEntry').mockResolvedValue(saved)
    mount(EntryFormDialog, {
      attachTo: document.body,
      props: { open: true, kind: 'expense', gigId: null, editId: null },
    })
    await flushPromises()
    await setField('#ee-amount', '50')
    await setField('#ee-currency', 'EUR')
    await setField('#ee-date', '2026-09-15')
    await setField('#ee-notes', 'New cable')
    submitForm()
    await flushPromises()
    expect(spy).toHaveBeenCalledTimes(1)
    expect(spy.mock.calls[0]![0]).toMatchObject({
      kind: 'expense', amount_minor: 5000, currency: 'EUR', category: 'gear',
      entry_date: '2026-09-15', notes: 'New cable',
    })
  })

  it('rejects an invalid calendar date and disables the save button', async () => {
    const store = useEarningsStore()
    const spy = vi.spyOn(store, 'createEntry')
    const wrapper = mount(EntryFormDialog, {
      attachTo: document.body,
      props: { open: true, kind: 'income', gigId: null, editId: null },
    })
    await flushPromises()
    await setField('#ee-amount', '100')
    await setField('#ee-description', 'Some income')
    // JSDOM strips bad dates from <input type="date"> before our model can
    // observe them, so drive the form model directly through the input's
    // prototype — that keeps the raw 2026-02-30 string in `form.entry_date`
    // so the calendar validator runs.
    const dateInput = document.querySelector('#ee-date') as HTMLInputElement
    Object.defineProperty(dateInput, 'value', {
      configurable: true,
      get: () => '2026-02-30',
      set: () => undefined,
    })
    dateInput.dispatchEvent(new Event('input', { bubbles: true }))
    dateInput.dispatchEvent(new Event('change', { bubbles: true }))
    await flushPromises()
    const msg = msgFor('#ee-date')
    expect(msg.classList.contains('ee-error')).toBe(true)
    expect(msg.textContent ?? '').toMatch(/calendar/i)
    expect(findSaveButton().disabled).toBe(true)
    submitForm()
    await flushPromises()
    expect(spy).not.toHaveBeenCalled()
    void wrapper
  })

  it('rejects an empty category and keeps the save button disabled', async () => {
    const store = useEarningsStore()
    const spy = vi.spyOn(store, 'createEntry')
    mount(EntryFormDialog, {
      attachTo: document.body,
      props: { open: true, kind: 'income', gigId: null, editId: null },
    })
    await flushPromises()
    await setField('#ee-amount', '100')
    await setField('#ee-description', 'Some income')
    // Pick the disabled empty placeholder option. Vue's v-model on <select>
    // iterates `el.options` and pushes whatever carries `selected === true`,
    // so marking the `value=""` placeholder selected is enough to drive
    // `form.category` to '' without fighting the prototype-level `value`
    // getter that JSDOM exposes.
    const select = document.querySelector('#ee-category') as HTMLSelectElement
    const emptyOption = Array.from(select.options).find((o) => o.value === '')
    expect(emptyOption).toBeDefined()
    emptyOption!.selected = true
    select.dispatchEvent(new Event('change', { bubbles: true }))
    await flushPromises()
    const msg = msgFor('#ee-category')
    expect(msg.classList.contains('ee-error')).toBe(true)
    expect(msg.textContent ?? '').toMatch(/pick a category/i)
    expect(findSaveButton().disabled).toBe(true)
    submitForm()
    await flushPromises()
    expect(spy).not.toHaveBeenCalled()
  })

  it('surfaces the FIN-10 notice copy in the dialog description', async () => {
    mount(EntryFormDialog, {
      attachTo: document.body,
      props: { open: true, kind: 'income', gigId: null, editId: null },
    })
    await flushPromises()
    // The dialog surfaces the FIN-10 disclaimer inside its body so the
    // user knows no tax / VAT is computed on this entry.
    const text = document.body.textContent ?? ''
    expect(text).toMatch(/FIN-10/)
  })

  it('round-trips a JPY entry unchanged through edit (no 100x data loss)', async () => {
    // Regression: loadFromEntry used Math.round(e.amount_minor / 100) for
    // every currency, which silently dropped the last two digits for 0-decimal
    // currencies like JPY. Editing a ¥1200 entry prefilled "12" and a save
    // would POST amount_minor = 12. The fix uses minorToDecimalString so
    // JPY stays "1200" → amount_minor 1200 on save.
    const store = useEarningsStore()
    const original = makeEntry({
      id: 'jpy-1',
      amount_minor: 1200,
      currency: 'JPY',
      category: 'tip',
      description: 'Tokyo gig tip',
      entry_date: '2026-09-15',
    })
    store.entries = [original]
    const updated = { ...original, amount_minor: 1200, currency: 'JPY' }
    const spy = vi.spyOn(store, 'updateEntry').mockResolvedValue(updated)
    mount(EntryFormDialog, {
      attachTo: document.body,
      props: { open: true, kind: 'income', gigId: null, editId: 'jpy-1' },
    })
    await flushPromises()
    // The amount input must be prefilled with the JPY major-unit string,
    // not the EUR-scaled "12" that the previous /100 division produced.
    const amountInput = document.querySelector('#ee-amount') as HTMLInputElement
    expect(amountInput.value).toBe('1200')
    // Currency code echoes through to the input as well (the CURRENCY_OPTIONS
    // datalist includes JPY) — sanity-check the field state.
    expect((document.querySelector('#ee-currency') as HTMLInputElement).value).toBe('JPY')
    // Submitting without further edits must POST amount_minor = 1200
    // unchanged. We do not touch the amount input so any drift in the
    // loader (e.g. dropping a digit) would surface here.
    submitForm()
    await flushPromises()
    expect(spy).toHaveBeenCalledTimes(1)
    // updateEntry(id, input) — the request body is the second argument.
    expect(spy.mock.calls[0]![1]).toMatchObject({
      kind: 'income', amount_minor: 1200, currency: 'JPY', category: 'tip',
      entry_date: '2026-09-15', description: 'Tokyo gig tip',
    })
  })
})