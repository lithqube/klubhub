import { describe, it, expect } from 'vitest'
import {
  countryError,
  fieldId,
  focusField,
  inlineErrorMessage,
  prefixError,
  problemTarget,
  problemsByField,
} from '../invoiceFields'
import { buildGigChoices, matchesGigQuery } from '../invoiceGigChoices'

describe('invoiceFields', () => {
  it('derives stable DOM ids from field paths', () => {
    expect(fieldId('customer.vat_id')).toBe('invf-customer-vat_id')
    expect(fieldId('tax_rate_bps')).toBe('invf-tax_rate_bps')
  })

  it('focuses the input for a problem, false when not rendered', () => {
    document.body.innerHTML = `<input id="${fieldId('customer.city')}">`
    expect(focusField('customer.city')).toBe(true)
    expect(document.activeElement?.id).toBe('invf-customer-city')
    expect(focusField('customer.country')).toBe(false)
  })

  it('routes supplier and total problems away from the draft form', () => {
    expect(problemTarget('supplier.vat_id')).toBe('billing_profile')
    expect(problemTarget('total_minor')).toBe('gig')
    expect(problemTarget('customer.city')).toBe('field')
  })

  it('keeps the first message per field', () => {
    expect(problemsByField([
      { field: 'customer.country', message: 'a' },
      { field: 'customer.country', message: 'b' },
    ])).toEqual({ 'customer.country': 'a' })
  })

  it('mirrors the API prefix rule', () => {
    expect(prefixError('INV')).toBe('')
    expect(prefixError('inv2026')).toBe('')
    expect(prefixError('')).toMatch(/1 to 20/)
    expect(prefixError('A'.repeat(21))).toMatch(/1 to 20/)
    expect(prefixError('INV-')).toMatch(/letters and digits/)
    expect(prefixError('cn')).toMatch(/reserved/)
  })

  it('accepts empty or two-letter countries only', () => {
    expect(countryError('')).toBe('')
    expect(countryError('de')).toBe('')
    expect(countryError('DEU')).not.toBe('')
  })

  it('leaves 409s to the banner and explains the rest', () => {
    expect(inlineErrorMessage({ code: 'conflict', message: 'x' })).toBeNull()
    expect(inlineErrorMessage({ code: 'bad_state', message: 'x' })).toBeNull()
    expect(inlineErrorMessage({ code: 'exceeds_balance', message: 'x' })).toMatch(/balance/)
    expect(inlineErrorMessage({ code: 'validation_failed', message: 'validation failed: x' })).toBe('validation failed: x')
  })
})

describe('buildGigChoices', () => {
  const gigs = [
    { id: 'g1', date: '2026-08-01T00:00:00Z', label: 'Club Alpha', status: 'inquiry', fee_amount: 500, fee_currency: 'EUR' },
    { id: 'g2', date: '2026-08-10T00:00:00Z', label: 'Club Beta', status: 'played', fee_amount: 800, fee_currency: 'EUR' },
    { id: 'g3', date: '2026-09-10T00:00:00Z', label: 'Club Gamma', status: 'confirmed', fee_amount: 0, fee_currency: 'USD' },
    { id: 'g4', date: '2026-09-01T00:00:00Z', label: 'Club Delta', status: 'advanced', fee_amount: 1200, fee_currency: 'EUR' },
  ]

  it('lists billable gigs first, invoiceable before disabled, newest first', () => {
    const out = buildGigChoices(gigs, { g4: { invoice_number: 'INV-0001-EUR', kind: 'invoice' } })
    expect(out.map((c) => c.id)).toEqual(['g2', 'g3', 'g4', 'g1'])
    expect(out.find((c) => c.id === 'g3')?.disabledReason).toBe('No fee set on the gig')
    expect(out.find((c) => c.id === 'g4')?.disabledReason).toBe('Already invoiced (INV-0001-EUR)')
    expect(out.find((c) => c.id === 'g1')?.group).toBe('other')
    expect(out.find((c) => c.id === 'g2')?.feeMinor).toBe(80000)
  })

  it('names an unnumbered active invoice as a draft', () => {
    const out = buildGigChoices(gigs, { g2: { invoice_number: null, kind: 'invoice' } })
    expect(out.find((c) => c.id === 'g2')?.disabledReason).toBe('Already invoiced (DRAFT)')
  })

  it('matches queries on label, date and status', () => {
    const [c] = buildGigChoices([gigs[1]!], {})
    expect(matchesGigQuery(c!, 'beta')).toBe(true)
    expect(matchesGigQuery(c!, 'played')).toBe(true)
    expect(matchesGigQuery(c!, '2026-08-10')).toBe(true)
    expect(matchesGigQuery(c!, 'gamma')).toBe(false)
  })
})
