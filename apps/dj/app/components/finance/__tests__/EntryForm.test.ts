// EntryForm — TDD coverage for FIN-01 / FIN-02 / FIN-10.
// Verifies the inline form actually saves income & expense rows, pre-loads the
// gig id, rejects invalid calendar dates, refuses empty categories, and
// surfaces the FIN-10 notice copy. The dialog mirrors the API contract:
// the date is a real calendar day (no 2026-02-30) and the required-string
// branch flips between description (income) and notes (expense).
//
// The form renders inline (no portal); `document.querySelector` still finds
// its fields because the wrapper is attached to document.body.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import EntryForm from '../EntryForm.vue'
import { useEarningsStore } from '../../../stores/earnings'
import type { Entry } from '../../../types/finance'
import { makeEntry as makeExpense } from './fixtures'

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
    attachment_count: 0,
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

describe('EntryForm', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    document.body.innerHTML = ''
  })

  it('saves a happy-path income entry and forwards the gigId branch', async () => {
    const store = useEarningsStore()
    const saved = makeEntry({ id: 'new', amount_minor: 150000, description: 'Sample Records — Royalties' })
    const spy = vi.spyOn(store, 'createEntry').mockResolvedValue(saved)
    const gigId = '00000000-0000-4000-8000-000000000001'
    const wrapper = mount(EntryForm, {
      attachTo: document.body,
      props: { kind: 'income', gigId, editId: null },
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
    expect(wrapper.emitted('saved')?.[0]).toEqual([saved])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('saves a happy-path expense entry using the notes field', async () => {
    const store = useEarningsStore()
    const saved = makeEntry({ id: 'new-e', kind: 'expense', amount_minor: 5000, notes: 'New cable' })
    const spy = vi.spyOn(store, 'createEntry').mockResolvedValue(saved)
    mount(EntryForm, {
      attachTo: document.body,
      props: { kind: 'expense', gigId: null, editId: null },
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
    const wrapper = mount(EntryForm, {
      attachTo: document.body,
      props: { kind: 'income', gigId: null, editId: null },
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
    mount(EntryForm, {
      attachTo: document.body,
      props: { kind: 'income', gigId: null, editId: null },
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

  it('surfaces the FIN-10 notice copy in the inline form', async () => {
    mount(EntryForm, {
      attachTo: document.body,
      props: { kind: 'income', gigId: null, editId: null },
    })
    await flushPromises()
    // The form surfaces the FIN-10 disclaimer inside its body so the
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
    mount(EntryForm, {
      attachTo: document.body,
      props: { kind: 'income', gigId: null, editId: 'jpy-1' },
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
describe('EntryForm as an inline composer (not a modal)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    document.body.innerHTML = ''
  })

  it('is a labelled region named by its title, not a dialog', async () => {
    mount(EntryForm, { attachTo: document.body, props: { kind: 'expense', gigId: null, editId: null } })
    await flushPromises()
    const region = document.querySelector('section.ee') as HTMLElement
    expect(region).not.toBeNull()
    expect(region.getAttribute('aria-labelledby')).toBe('ee-title')
    expect(document.getElementById('ee-title')!.textContent).toBe('NEW EXPENSE')
    expect(document.querySelector('[role="dialog"], [aria-modal="true"]')).toBeNull()
  })

  it('titles follow the kind and the mode', async () => {
    const store = useEarningsStore()
    store.entries = [makeExpense({ id: 'x', kind: 'income', description: 'DJ set', notes: '' })]
    const income = mount(EntryForm, { attachTo: document.body, props: { kind: 'income', gigId: null, editId: null } })
    expect(document.getElementById('ee-title')!.textContent).toBe('NEW INCOME')
    income.unmount()
    mount(EntryForm, { attachTo: document.body, props: { kind: 'expense', gigId: null, editId: 'x' } })
    // In edit mode the entry's own kind wins over the create preset.
    expect(document.getElementById('ee-title')!.textContent).toBe('EDIT INCOME')
  })

  it('moves focus to the first field when it opens', async () => {
    mount(EntryForm, { attachTo: document.body, props: { kind: 'income', gigId: null, editId: null } })
    await vi.waitFor(() => expect(document.activeElement).toBe(document.querySelector('#ee-amount')))
  })

  it('CANCEL closes it and saves nothing', async () => {
    const store = useEarningsStore()
    const create = vi.spyOn(store, 'createEntry')
    const wrapper = mount(EntryForm, { attachTo: document.body, props: { kind: 'income', gigId: null, editId: null } })
    await setField('#ee-amount', '100')
    ;(Array.from(document.querySelectorAll('button')).find((b) => b.textContent?.trim() === 'CANCEL') as HTMLButtonElement).click()
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(create).not.toHaveBeenCalled()
  })

  it('Escape closes it', async () => {
    const wrapper = mount(EntryForm, { attachTo: document.body, props: { kind: 'income', gigId: null, editId: null } })
    await flushPromises()
    document.querySelector('#ee-amount')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('Escape inside the category list closes the list, not the form', async () => {
    const wrapper = mount(EntryForm, { attachTo: document.body, props: { kind: 'income', gigId: null, editId: null } })
    await flushPromises()
    document.querySelector('#ee-category')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('asks for focus on the first field again when the store re-requests the composer', async () => {
    const wrapper = mount(EntryForm, { attachTo: document.body, props: { kind: 'income', gigId: null, editId: null, focusSeq: 1 } })
    await vi.waitFor(() => expect(document.activeElement).toBe(document.querySelector('#ee-amount')))
    ;(document.querySelector('#ee-gig') as HTMLInputElement).focus()
    await wrapper.setProps({ focusSeq: 2 })
    await vi.waitFor(() => expect(document.activeElement).toBe(document.querySelector('#ee-amount')))
  })

  it('keeps the draft when the same kind is requested again', async () => {
    const wrapper = mount(EntryForm, { attachTo: document.body, props: { kind: 'expense', gigId: null, editId: null, focusSeq: 1 } })
    await setField('#ee-notes', 'Half-typed')
    await wrapper.setProps({ focusSeq: 2 })
    expect((document.querySelector('#ee-notes') as HTMLTextAreaElement).value).toBe('Half-typed')
  })

  it('gives focus back to the control that opened it when it goes away', async () => {
    const opener = document.createElement('button')
    opener.textContent = '+ NEW EXPENSE'
    document.body.appendChild(opener)
    opener.focus()
    const wrapper = mount(EntryForm, { attachTo: document.body, props: { kind: 'expense', gigId: null, editId: null } })
    await vi.waitFor(() => expect(document.activeElement).toBe(document.querySelector('#ee-amount')))
    wrapper.unmount()
    await vi.waitFor(() => expect(document.activeElement).toBe(opener))
  })

  describe('validation messages', () => {
    const blur = async (selector: string): Promise<void> => {
      document.querySelector(selector)!.dispatchEvent(new Event('blur'))
      await flushPromises()
    }
    const isError = (selector: string): boolean => msgFor(selector).classList.contains('ee-error')
    const invalid = (selector: string): boolean => document.querySelector(selector)!.getAttribute('aria-invalid') === 'true'

    it('a fresh form is quiet: no red, no aria-invalid, neutral hints, SAVE off with a reason', async () => {
      mount(EntryForm, { attachTo: document.body, props: { kind: 'expense', gigId: null, editId: null } })
      await flushPromises()
      for (const field of ['#ee-amount', '#ee-notes', '#ee-currency', '#ee-date', '#ee-category']) {
        expect(isError(field), `${field} should not be red`).toBe(false)
        expect(invalid(field), `${field} should not be aria-invalid`).toBe(false)
      }
      expect(document.querySelector('.ee-error')).toBeNull()
      // The hints still say what is needed, in the neutral voice.
      expect(msgFor('#ee-amount').textContent).toContain('Positive number')
      expect(msgFor('#ee-notes').textContent).toBe('Required for expense entries.')
      // SAVE is disabled and the form says why.
      expect((document.querySelector('button[type="submit"]') as HTMLButtonElement).disabled).toBe(true)
      expect(document.querySelector('.ee-need')?.textContent).toContain('Fill in the fields marked *')
    })

    it('shows a field\'s error only once it has been left, and clears it when fixed', async () => {
      mount(EntryForm, { attachTo: document.body, props: { kind: 'expense', gigId: null, editId: null } })
      await flushPromises()

      await blur('#ee-amount')
      expect(isError('#ee-amount')).toBe(true)
      expect(msgFor('#ee-amount').textContent).toBe('Add the amount.')
      expect(invalid('#ee-amount')).toBe(true)
      // Other fields that were not left stay quiet.
      expect(isError('#ee-notes')).toBe(false)

      await setField('#ee-amount', '42.50')
      expect(isError('#ee-amount')).toBe(false)
      expect(invalid('#ee-amount')).toBe(false)
    })

    it('flags a bad amount after the user leaves the field, and a missing note after leaving the notes', async () => {
      mount(EntryForm, { attachTo: document.body, props: { kind: 'expense', gigId: null, editId: null } })
      await setField('#ee-amount', 'abc')
      expect(isError('#ee-amount')).toBe(false) // still typing
      await blur('#ee-amount')
      expect(msgFor('#ee-amount').textContent).toContain('positive number')

      await blur('#ee-notes')
      expect(isError('#ee-notes')).toBe(true)
      expect(msgFor('#ee-notes').textContent).toBe('Required for expense entries.')
    })

    it('stays disabled until every rule passes, and the reason line goes away with them', async () => {
      mount(EntryForm, { attachTo: document.body, props: { kind: 'expense', gigId: null, editId: null } })
      await flushPromises()
      const save = () => document.querySelector('button[type="submit"]') as HTMLButtonElement
      await setField('#ee-amount', '42.50')
      expect(save().disabled).toBe(true)
      await setField('#ee-notes', 'Strings')
      expect(save().disabled).toBe(false)
      expect(document.querySelector('.ee-need')).toBeNull()
    })
  })
})
