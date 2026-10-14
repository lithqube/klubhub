import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import BillingProfileForm from '../BillingProfileForm.vue'
import { DEFAULT_BILLING_PROFILE } from '../../../../shared/finance-mock/rules'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Call = [string, { method?: string; body?: Record<string, any> } | undefined]
const fetchMock = vi.fn()
const httpError = (status: number, body: unknown) => Object.assign(new Error(`HTTP ${status}`), { statusCode: status, data: body })
const puts = () => (fetchMock.mock.calls as Call[]).filter(([, o]) => o?.method === 'PUT')

let wrapper: VueWrapper | undefined
const q = (sel: string) => document.querySelector(sel) as HTMLInputElement
async function type(sel: string, value: string) {
  const el = q(sel)
  expect(el, sel).not.toBeNull()
  el.value = value
  el.dispatchEvent(new Event('input', { bubbles: true }))
  await flushPromises()
}
async function submit() {
  document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await flushPromises()
}
const saveButton = () => Array.from(document.querySelectorAll('button')).find((b) => /SAVE PROFILE|SAVING/.test(b.textContent ?? '')) as HTMLButtonElement

async function mountForm(profile = { ...DEFAULT_BILLING_PROFILE }) {
  fetchMock.mockImplementation(async (_u: string, o?: { method?: string }) => (o?.method === 'PUT' ? { data: profile } : { data: profile }))
  wrapper = mount(BillingProfileForm, { attachTo: document.body })
  await flushPromises()
}

beforeEach(() => {
  setActivePinia(createPinia())
  document.body.replaceChildren()
  fetchMock.mockReset()
  vi.stubGlobal('$fetch', fetchMock)
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.unstubAllGlobals() })

describe('BillingProfileForm', () => {
  it('loads the profile and has labelled Steuernummer, IBAN and BIC inputs with their values', async () => {
    await mountForm({ ...DEFAULT_BILLING_PROFILE, tax_number: '21/815/08150', iban: 'DE89370400440532013000', bic: 'COBADEFFXXX' })
    expect(document.querySelector('label[for="bp-tax_number"]')?.textContent).toMatch(/STEUERNUMMER/)
    expect(document.querySelector('label[for="bp-iban"]')?.textContent).toMatch(/IBAN/)
    expect(document.querySelector('label[for="bp-bic"]')?.textContent).toMatch(/BIC/)
    expect(q('#bp-tax_number').value).toBe('21/815/08150')
    expect(q('#bp-iban').value).toBe('DE89370400440532013000')
    expect(q('#bp-bic').value).toBe('COBADEFFXXX')
    expect(saveButton().disabled).toBe(true)
  })

  it('PUTs every profile field with the new ones as typed (spaces allowed) and the version token', async () => {
    await mountForm()
    await type('#bp-tax_number', ' 21/815/08150 ')
    await type('#bp-iban', 'DE89 3704 0044 0532 0130 00')
    await type('#bp-bic', 'cobadeffxxx')
    expect(saveButton().disabled).toBe(false)
    await submit()

    expect(puts()).toHaveLength(1)
    const body = puts()[0]![1]!.body!
    expect(body).toMatchObject({
      tax_number: '21/815/08150',
      iban: 'DE89 3704 0044 0532 0130 00',
      bic: 'cobadeffxxx',
      legal_name: 'Sam Example',
      default_vat_rate_bps: 1900,
      updated_at: DEFAULT_BILLING_PROFILE.updated_at,
    })
    expect(body).not.toHaveProperty('id')
  })

  it('shows the server normalisation after saving', async () => {
    await mountForm()
    fetchMock.mockImplementation(async () => ({
      data: { ...DEFAULT_BILLING_PROFILE, iban: 'DE89370400440532013000', updated_at: '2026-09-25T11:00:00Z' },
    }))
    await type('#bp-iban', 'de89 3704 0044 0532 0130 00')
    await submit()
    expect(q('#bp-iban').value).toBe('DE89370400440532013000')
    expect(document.body.textContent).toContain('Billing profile saved.')
    expect(saveButton().disabled).toBe(true)
  })

  it('puts each server field error on its own input and focuses the first', async () => {
    await mountForm()
    await type('#bp-iban', 'DE00')
    await type('#bp-bic', 'ABC')
    fetchMock.mockRejectedValueOnce(httpError(400, {
      error: 'validation_failed',
      message: 'validation failed: iban: not a valid IBAN (check the country, length and check digits); bic: must be an 8 or 11 character BIC/SWIFT code',
    }))
    await submit()

    expect(q('#bp-iban').getAttribute('aria-invalid')).toBe('true')
    expect(document.querySelector('#bp-iban-msg')?.textContent).toContain('not a valid IBAN')
    expect(q('#bp-bic').getAttribute('aria-invalid')).toBe('true')
    expect(document.querySelector('#bp-bic-msg')?.textContent).toContain('8 or 11 character')
    expect(q('#bp-tax_number').getAttribute('aria-invalid')).toBeNull()
    expect(document.activeElement?.id).toBe('bp-iban')
    // What the user typed stays so they can fix it.
    expect(q('#bp-iban').value).toBe('DE00')
  })

  it('shows a tax_number error from the server on the tax number input', async () => {
    await mountForm()
    await type('#bp-tax_number', 'x')
    fetchMock.mockRejectedValueOnce(httpError(400, { error: 'validation_failed', message: 'validation failed: tax_number: exceeds 40 characters' }))
    await submit()
    expect(document.querySelector('#bp-tax_number-msg')?.textContent).toContain('exceeds 40 characters')
  })

  it('offers a reload instead of a blind retry on a version conflict', async () => {
    await mountForm()
    await type('#bp-bic', 'COBADEFFXXX')
    fetchMock.mockRejectedValueOnce(httpError(409, { error: 'conflict', message: 'billing profile updated by another writer; refresh and retry' }))
    await submit()
    expect(document.body.textContent).toMatch(/changed elsewhere/i)
    expect(Array.from(document.querySelectorAll('button')).some((b) => b.textContent?.trim() === 'RELOAD')).toBe(true)
  })

  it('does not submit an invalid VAT rate', async () => {
    await mountForm()
    await type('#bp-default_vat_rate', 'abc')
    expect(saveButton().disabled).toBe(true)
    expect(document.querySelector('#bp-default_vat_rate-msg')?.textContent).toMatch(/0 to 100/)
  })

  it('explains when the server has no billing profile', async () => {
    fetchMock.mockRejectedValue(httpError(404, { error: 'not_found', message: 'billing profile not found' }))
    wrapper = mount(BillingProfileForm, { attachTo: document.body })
    await flushPromises()
    expect(document.body.textContent).toMatch(/No billing profile exists/)
    expect(document.querySelector('form')).toBeNull()
  })
})
