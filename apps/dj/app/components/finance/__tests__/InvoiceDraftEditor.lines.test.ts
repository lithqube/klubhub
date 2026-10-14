// Draft editor: multi-line editing, EN 16931 references, and the PUT payload.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import InvoiceDraftEditor from '../InvoiceDraftEditor.vue'
import { useInvoiceStore } from '../../../stores/invoice'
import type { Invoice, InvoiceLine } from '../../../types/finance'
import { makeInvoice } from './fixtures'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Call = [string, { method?: string; body?: Record<string, any> } | undefined]
const fetchMock = () => vi.mocked($fetch as unknown as (url: string, o?: unknown) => Promise<unknown>)
const putCalls = () => (fetchMock().mock.calls as Call[]).filter(([, o]) => o?.method === 'PUT')

function line(over: Partial<InvoiceLine> = {}): InvoiceLine {
  return {
    id: 'l1', invoice_id: 'inv-1', sort_order: 0, description: 'DJ set', quantity: 1, unit_minor: 80000,
    unit_code: 'C62', tax_bps: 1900, line_total_minor: 80000, created_at: '2026-09-25T10:00:00Z', ...over,
  }
}
const SAVED_LINES = [
  line(),
  line({ id: 'l2', sort_order: 1, description: 'Travel', quantity: 120, unit_minor: 35, unit_code: 'KMT', line_total_minor: 4200 }),
]

function draft(over: Partial<Invoice> = {}): Invoice {
  return makeInvoice({
    status: 'draft', invoice_number: null, customer: { ...makeInvoice().customer, country: 'DE' },
    updated_at: '2026-09-15T10:00:00Z', ...over,
  })
}

let wrapper: VueWrapper | undefined
function mountEditor(inv = draft(), lines = SAVED_LINES) {
  const store = useInvoiceStore()
  store.current = inv
  store.lines = lines
  const Parent = defineComponent({ setup: () => () => (store.current ? h(InvoiceDraftEditor, { invoice: store.current }) : null) })
  wrapper = mount(Parent, {
    attachTo: document.body,
    global: { stubs: { InvoicePartyFields: true, InvoiceTaxFields: true, InvoiceIssueChecklist: true } },
  })
  return { store, w: wrapper }
}

const q = (sel: string) => document.querySelector(sel) as HTMLInputElement
async function type(sel: string, value: string) {
  const el = q(sel)
  expect(el, sel).not.toBeNull()
  el.value = value
  el.dispatchEvent(new Event('input', { bubbles: true }))
  await flushPromises()
}
const saveButton = () => Array.from(document.querySelectorAll('button')).find((b) => /SAVE DRAFT|SAVING/.test(b.textContent ?? '')) as HTMLButtonElement
async function submit() {
  document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await flushPromises()
}
function httpError(status: number, body: unknown) {
  return Object.assign(new Error(`HTTP ${status}`), { statusCode: status, data: body })
}

beforeEach(() => {
  setActivePinia(createPinia())
  document.body.replaceChildren()
  vi.stubGlobal('$fetch', vi.fn())
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.unstubAllGlobals()
})

describe('InvoiceDraftEditor lines', () => {
  it('shows the saved lines as editable rows in major units and starts clean', () => {
    mountEditor()
    expect(q('#invf-lines-0-description').value).toBe('DJ set')
    expect(q('#invf-lines-0-unit_minor').value).toBe('800.00')
    expect(q('#invf-lines-1-description').value).toBe('Travel')
    expect(q('#invf-lines-1-quantity').value).toBe('120')
    expect((q('#invf-lines-1-unit_code') as unknown as HTMLSelectElement).value).toBe('KMT')
    expect(q('#invf-lines-1-unit_minor').value).toBe('0.35')
    expect(saveButton().disabled).toBe(true)
    expect(document.body.textContent).toContain('All changes saved.')
  })

  it('PUTs the FULL line list in minor units, without totals or tax, and with the original token', async () => {
    mountEditor()
    fetchMock().mockImplementation(async (_url, o) => ((o as { method?: string } | undefined)?.method === 'PUT' ? { data: draft() } : { data: draft(), lines: SAVED_LINES }))
    await type('#invf-lines-1-unit_minor', '0.40')
    await submit()

    expect(putCalls()).toHaveLength(1)
    const body = putCalls()[0]![1]!.body!
    expect(body.updated_at).toBe('2026-09-15T10:00:00Z')
    expect(body.lines).toEqual([
      { description: 'DJ set', quantity: 1, unit_minor: 80000, unit_code: 'C62' },
      { description: 'Travel', quantity: 120, unit_minor: 40, unit_code: 'KMT' },
    ])
    for (const l of body.lines) expect(Object.keys(l)).not.toEqual(expect.arrayContaining(['line_total_minor']))
    expect(body).not.toHaveProperty('subtotal_minor')
  })

  it('omits `lines` when only another field changed, but always sends the four references', async () => {
    mountEditor(draft({ buyer_reference: '04011000-12345-34', payment_terms: 'Payable within 14 days' }))
    fetchMock().mockResolvedValue({ data: draft() })
    await type('#invf-purchase_order_ref', '  PO-7 ')
    await submit()

    const body = putCalls()[0]![1]!.body!
    expect(body).not.toHaveProperty('lines')
    expect(body).toMatchObject({
      buyer_reference: '04011000-12345-34',
      purchase_order_ref: 'PO-7',
      contract_ref: '',
      payment_terms: 'Payable within 14 days',
    })
  })

  it('labels the buyer reference as the Leitweg-ID for public-sector e-invoices', () => {
    mountEditor()
    expect(document.querySelector('label[for="invf-buyer_reference"]')?.textContent).toMatch(/Leitweg-ID/i)
    expect(document.querySelector('#invf-buyer_reference-msg')?.textContent).toMatch(/public-sector/i)
  })

  it('blocks saving while an edited line is invalid and says why', async () => {
    mountEditor()
    await type('#invf-lines-0-quantity', '0')
    expect(saveButton().disabled).toBe(true)
    expect(document.body.textContent).toContain('Fix the line items to save.')
    await submit()
    expect(putCalls()).toHaveLength(0)
  })

  it('lets a draft with no saved lines save other fields without sending placeholder lines', async () => {
    mountEditor(draft(), [])
    expect(q('#invf-lines-0-description').value).toBe('')
    fetchMock().mockResolvedValue({ data: draft() })
    await type('#invf-contract_ref', 'C-1')
    expect(saveButton().disabled).toBe(false)
    await submit()
    expect(putCalls()[0]![1]!.body).not.toHaveProperty('lines')
  })

  it('shows a per-line server error on the offending input and focuses it', async () => {
    mountEditor()
    fetchMock().mockRejectedValueOnce(httpError(400, {
      error: 'validation_failed',
      message: 'validation failed: lines[1].quantity: must be between 1 and 1000000; lines[0].unit_minor: must not be negative',
    }))
    await type('#invf-lines-1-quantity', '5')
    await submit()

    expect(q('#invf-lines-1-quantity').getAttribute('aria-invalid')).toBe('true')
    expect(document.querySelector('#invf-lines-1-quantity-msg')?.textContent).toContain('between 1 and 1000000')
    expect(q('#invf-lines-0-unit_minor').getAttribute('aria-invalid')).toBe('true')
    expect(document.activeElement?.id).toBe('invf-lines-1-quantity')
    expect(document.body.textContent).toContain('and 1 more')
    // The user's edit is kept so they can correct it and retry.
    expect(q('#invf-lines-1-quantity').value).toBe('5')
  })

  it('shows a server error on a reference field', async () => {
    mountEditor()
    fetchMock().mockRejectedValueOnce(httpError(400, { error: 'validation_failed', message: 'validation failed: buyer_reference: exceeds 100 characters' }))
    await type('#invf-buyer_reference', 'x')
    await submit()
    expect(q('#invf-buyer_reference').getAttribute('aria-invalid')).toBe('true')
    expect(document.querySelector('#invf-buyer_reference-msg')?.textContent).toContain('exceeds 100 characters')
  })

  it('reloads the lines after a save that replaced them, so the rows match what the server stored', async () => {
    const { store } = mountEditor()
    const stored = [line({ id: 'n1', description: 'Resident set', quantity: 3, unit_minor: 25000, unit_code: 'HUR', line_total_minor: 75000 })]
    fetchMock().mockImplementation(async (_url, o) => ((o as { method?: string } | undefined)?.method === 'PUT'
      ? { data: draft({ updated_at: '2026-09-15T11:00:00Z', subtotal_minor: 75000 }) }
      : { data: draft({ updated_at: '2026-09-15T11:00:00Z' }), lines: stored }))
    await type('#invf-lines-0-description', 'Resident set')
    await submit()

    expect(store.lines).toEqual(stored)
    expect(q('#invf-lines-0-description').value).toBe('Resident set')
    expect(q('#invf-lines-0-unit_minor').value).toBe('250.00')
    expect(document.querySelector('#invf-lines-1-description')).toBeNull()
    expect(document.body.textContent).toContain('All changes saved.')
  })

  it('removing a row sends the remaining rows as the new full list', async () => {
    mountEditor()
    fetchMock().mockImplementation(async (_url, o) => ((o as { method?: string } | undefined)?.method === 'PUT' ? { data: draft() } : { data: draft(), lines: [SAVED_LINES[0]!] }))
    ;(document.querySelector('button[aria-label="Remove line 2"]') as HTMLButtonElement).click()
    await flushPromises()
    await submit()
    expect(putCalls()[0]![1]!.body!.lines).toEqual([{ description: 'DJ set', quantity: 1, unit_minor: 80000, unit_code: 'C62' }])
  })
})
