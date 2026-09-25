import { describe, it, expect } from 'vitest'
import {
  VAT_TREATMENTS,
  VAT_TREATMENT_ORDER,
  applyTreatmentChange,
  isVatTreatment,
  vatTreatmentLabel,
  vatTreatmentMeta,
} from '../vatTreatment'

describe('vatTreatment table', () => {
  it('covers every treatment exactly once in the display order', () => {
    expect([...VAT_TREATMENT_ORDER].sort()).toEqual(Object.keys(VAT_TREATMENTS).sort())
  })

  it('uses the contract default notes', () => {
    expect(VAT_TREATMENTS.reverse_charge.defaultNote).toBe(
      'Reverse charge: VAT to be accounted for by the recipient (Art. 196 Directive 2006/112/EC).',
    )
    expect(VAT_TREATMENTS.exempt.defaultNote).toBe('VAT exempt: small business scheme.')
    expect(VAT_TREATMENTS.outside_scope.defaultNote).toBe(
      'Outside the scope of EU VAT: place of supply outside the EU.',
    )
  })

  it('only domestic and US sales tax have an editable rate', () => {
    const editable = VAT_TREATMENT_ORDER.filter((t) => VAT_TREATMENTS[t].rateEditable)
    expect(editable).toEqual(['domestic', 'us_sales_tax'])
  })

  it('requires a note for exempt and outside_scope (issue rules)', () => {
    expect(VAT_TREATMENTS.exempt.requiresNote).toBe(true)
    expect(VAT_TREATMENTS.outside_scope.requiresNote).toBe(true)
    expect(VAT_TREATMENTS.none.requiresNote).toBe(false)
  })

  it('falls back to none for unknown values', () => {
    expect(vatTreatmentMeta('bogus').value).toBe('none')
    expect(vatTreatmentLabel('reverse_charge')).toBe('REVERSE CHARGE')
    expect(isVatTreatment('domestic')).toBe(true)
    expect(isVatTreatment('bogus')).toBe(false)
  })
})

describe('applyTreatmentChange', () => {
  it('forces the rate to 0 and sets the default note for fixed-rate treatments', () => {
    const next = applyTreatmentChange({ vat_treatment: 'domestic', tax_rate_bps: 1900, tax_note: '' }, 'reverse_charge')
    expect(next.tax_rate_bps).toBe(0)
    expect(next.tax_note).toBe(VAT_TREATMENTS.reverse_charge.defaultNote)
  })

  it('keeps the rate when switching to an editable treatment', () => {
    const next = applyTreatmentChange({ vat_treatment: 'domestic', tax_rate_bps: 1900, tax_note: '' }, 'us_sales_tax')
    expect(next.tax_rate_bps).toBe(1900)
    expect(next.tax_note).toBe('')
  })

  it('replaces a default note but keeps custom wording', () => {
    const fromDefault = applyTreatmentChange(
      { vat_treatment: 'exempt', tax_rate_bps: 0, tax_note: VAT_TREATMENTS.exempt.defaultNote },
      'outside_scope',
    )
    expect(fromDefault.tax_note).toBe(VAT_TREATMENTS.outside_scope.defaultNote)

    const custom = applyTreatmentChange(
      { vat_treatment: 'exempt', tax_rate_bps: 0, tax_note: 'Kleinunternehmer §19 UStG' },
      'outside_scope',
    )
    expect(custom.tax_note).toBe('Kleinunternehmer §19 UStG')
  })
})
