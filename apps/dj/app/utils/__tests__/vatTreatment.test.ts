import { describe, it, expect } from 'vitest'
import {
  VAT_TREATMENTS,
  VAT_TREATMENT_ORDER,
  applyTreatmentChange,
  isVatTreatment,
  vatTreatmentLabel,
  vatTreatmentMeta,
} from '../vatTreatment'

// The server owns the legal wording per supplier country (GET /invoices/tax-notes).
// When the user switches treatment the note must follow, and wording that was
// only ever a default (generic English or the country's) must never stay behind
// under a treatment it does not belong to.
describe('applyTreatmentChange with the supplier country\'s notes', () => {
  const DE = {
    exempt: 'Gemäß § 19 UStG wird keine Umsatzsteuer berechnet. / VAT is not charged under § 19 UStG.',
    reverse_charge: 'Steuerschuldnerschaft des Leistungsempfängers (Reverse Charge) / Reverse charge.',
    outside_scope: 'Nicht steuerbare sonstige Leistung (§ 3a Abs. 2 UStG).',
  } as const
  const from = (vat_treatment: 'domestic' | 'exempt' | 'reverse_charge' | 'outside_scope', tax_note: string) =>
    ({ vat_treatment, tax_rate_bps: 0, tax_note })

  it('moves to the country\'s wording for the new treatment', () => {
    expect(applyTreatmentChange(from('domestic', ''), 'exempt', DE).tax_note).toBe(DE.exempt)
    expect(applyTreatmentChange(from('domestic', ''), 'reverse_charge', DE).tax_note).toBe(DE.reverse_charge)
  })

  it('drops the country\'s wording when the new treatment carries none (no § 19 text left on a domestic invoice)', () => {
    expect(applyTreatmentChange(from('exempt', DE.exempt), 'domestic', DE).tax_note).toBe('')
    expect(applyTreatmentChange(from('reverse_charge', DE.reverse_charge), 'domestic', DE).tax_note).toBe('')
  })

  it('swaps one country default for another', () => {
    expect(applyTreatmentChange(from('exempt', DE.exempt), 'reverse_charge', DE).tax_note).toBe(DE.reverse_charge)
  })

  it('still recognises the generic English default of an older draft as replaceable', () => {
    const generic = VAT_TREATMENTS.exempt.defaultNote
    expect(applyTreatmentChange(from('exempt', generic), 'reverse_charge', DE).tax_note).toBe(DE.reverse_charge)
    expect(applyTreatmentChange(from('exempt', generic), 'domestic', DE).tax_note).toBe('')
  })

  it('keeps wording the user wrote themselves', () => {
    const own = 'Befreit nach Rücksprache mit dem Finanzamt.'
    expect(applyTreatmentChange(from('exempt', own), 'reverse_charge', DE).tax_note).toBe(own)
    expect(applyTreatmentChange(from('exempt', own), 'domestic', DE).tax_note).toBe(own)
  })

  it('falls back to the built-in defaults when the notes are not known yet', () => {
    expect(applyTreatmentChange(from('domestic', ''), 'exempt').tax_note).toBe(VAT_TREATMENTS.exempt.defaultNote)
    expect(applyTreatmentChange(from('domestic', ''), 'exempt', {}).tax_note).toBe(VAT_TREATMENTS.exempt.defaultNote)
    // A treatment the server sent nothing for keeps its built-in default.
    expect(applyTreatmentChange(from('domestic', ''), 'outside_scope', { exempt: DE.exempt }).tax_note)
      .toBe(VAT_TREATMENTS.outside_scope.defaultNote)
  })

  it('still forces the rate to 0 for fixed-rate treatments', () => {
    expect(applyTreatmentChange({ vat_treatment: 'domestic', tax_rate_bps: 1900, tax_note: '' }, 'exempt', DE).tax_rate_bps).toBe(0)
  })
})

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
