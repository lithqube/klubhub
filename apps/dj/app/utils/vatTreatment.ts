// Single source of truth for how each VAT treatment behaves in the UI.
// Components read this table instead of branching on treatment names, so a
// new treatment (or a country-specific note, docs/INVOICING.md §6) is a data
// change here, not a hunt through templates.

import type { VatTreatment } from '../types/finance'

export interface VatTreatmentMeta {
  value: VatTreatment
  /** Short uppercase label for selects and badges. */
  label: string
  /** One-line explanation shown under the select. */
  description: string
  /** Whether the user picks the rate; otherwise the rate is fixed at 0. */
  rateEditable: boolean
  /** Whether the rate must be above zero for the invoice to be issued. */
  requiresPositiveRate: boolean
  /** Whether an issued invoice must carry a legal note for this treatment. */
  requiresNote: boolean
  /** Default legal wording printed on the invoice ('' = none). */
  defaultNote: string
  /** Whether the customer's VAT ID is required to issue. */
  requiresCustomerVatId: boolean
}

export const VAT_TREATMENTS: Record<VatTreatment, VatTreatmentMeta> = {
  domestic: {
    value: 'domestic',
    label: 'DOMESTIC VAT',
    description: 'You charge VAT at your own country\'s rate.',
    rateEditable: true,
    requiresPositiveRate: true,
    requiresNote: false,
    defaultNote: '',
    requiresCustomerVatId: false,
  },
  reverse_charge: {
    value: 'reverse_charge',
    label: 'REVERSE CHARGE',
    description: 'EU business customer in another EU country accounts for the VAT. No VAT on this invoice.',
    rateEditable: false,
    requiresPositiveRate: false,
    requiresNote: true,
    defaultNote: 'Reverse charge: VAT to be accounted for by the recipient (Art. 196 Directive 2006/112/EC).',
    requiresCustomerVatId: true,
  },
  exempt: {
    value: 'exempt',
    label: 'VAT EXEMPT',
    description: 'You are VAT-exempt (e.g. small-business scheme). No VAT on this invoice.',
    rateEditable: false,
    requiresPositiveRate: false,
    requiresNote: true,
    defaultNote: 'VAT exempt: small business scheme.',
    requiresCustomerVatId: false,
  },
  outside_scope: {
    value: 'outside_scope',
    label: 'OUTSIDE EU VAT',
    description: 'Customer is outside the EU, so the service is outside the scope of EU VAT.',
    rateEditable: false,
    requiresPositiveRate: false,
    requiresNote: true,
    defaultNote: 'Outside the scope of EU VAT: place of supply outside the EU.',
    requiresCustomerVatId: false,
  },
  us_sales_tax: {
    value: 'us_sales_tax',
    label: 'US SALES TAX',
    description: 'You charge state/local sales tax at the rate you enter.',
    rateEditable: true,
    requiresPositiveRate: false,
    requiresNote: false,
    defaultNote: '',
    requiresCustomerVatId: false,
  },
  none: {
    value: 'none',
    label: 'NO TAX',
    description: 'No tax applies to this invoice.',
    rateEditable: false,
    requiresPositiveRate: false,
    requiresNote: false,
    defaultNote: '',
    requiresCustomerVatId: false,
  },
}

/** Display order for selects. */
export const VAT_TREATMENT_ORDER: VatTreatment[] = [
  'domestic',
  'reverse_charge',
  'exempt',
  'outside_scope',
  'us_sales_tax',
  'none',
]

export function vatTreatmentMeta(t: VatTreatment | string | null | undefined): VatTreatmentMeta {
  return VAT_TREATMENTS[(t as VatTreatment)] ?? VAT_TREATMENTS.none
}

export function vatTreatmentLabel(t: VatTreatment | string | null | undefined): string {
  return vatTreatmentMeta(t).label
}

export function isVatTreatment(v: unknown): v is VatTreatment {
  return typeof v === 'string' && v in VAT_TREATMENTS
}

/**
 * The tax fields after switching treatment: fixed-rate treatments force the
 * rate to 0, and the note moves to the new default unless the user had
 * written their own wording.
 */
export function applyTreatmentChange(
  current: { vat_treatment: VatTreatment; tax_rate_bps: number; tax_note: string },
  next: VatTreatment,
): { vat_treatment: VatTreatment; tax_rate_bps: number; tax_note: string } {
  const prevMeta = vatTreatmentMeta(current.vat_treatment)
  const nextMeta = vatTreatmentMeta(next)
  const noteIsDefault = current.tax_note.trim() === '' || current.tax_note === prevMeta.defaultNote
  return {
    vat_treatment: next,
    tax_rate_bps: nextMeta.rateEditable ? current.tax_rate_bps : 0,
    tax_note: noteIsDefault ? nextMeta.defaultNote : current.tax_note,
  }
}

/** Withholding bounds from the issue rules (0–50%). */
export const WITHHOLDING_MAX_BPS = 5000
