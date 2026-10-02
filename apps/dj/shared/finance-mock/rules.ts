// Pure invoicing rules (docs/INVOICING.md §3–§4.1) shared by the Nitro dev
// mocks (server/api/v1/finance) and the browser demo (app/demo). No I/O.

import type { BillingProfile, Party, TaxSuggestion, VatTreatment } from '../../app/types/finance'

/** Billing profile shape used by the mocks. */
export type Supplier = Party & { vat_exempt_small_business: boolean; default_vat_rate_bps: number }

/** Result of a mock request: HTTP status plus JSON body. */
export interface MockResult<T = unknown> {
  status: number
  body: T
}

export const ok = <T>(body: T, status = 200): MockResult<T> => ({ status, body })

export function fail(status: number, error: string, message: string, problems?: { field: string; message: string }[]): MockResult {
  return { status, body: problems ? { error, message, problems } : { error, message } }
}

export function badRequest(errs: string[]): MockResult {
  return fail(400, 'validation_failed', 'validation failed: ' + errs.join('; '))
}

/** Banker-free half-up rounding of minor units × basis points. */
export function applyBps(minor: number, bps: number): number {
  const product = minor * bps
  const q = Math.trunc(product / 10000)
  const r = product - q * 10000
  return Math.abs(r) * 2 >= 10000 ? q + Math.sign(product) : q
}

export const EU = new Set([
  'AT', 'BE', 'BG', 'HR', 'CY', 'CZ', 'DK', 'EE', 'FI', 'FR', 'DE', 'GR', 'HU', 'IE',
  'IT', 'LV', 'LT', 'LU', 'MT', 'NL', 'PL', 'PT', 'RO', 'SK', 'SI', 'ES', 'SE',
])

const TREATMENTS: VatTreatment[] = ['domestic', 'reverse_charge', 'exempt', 'outside_scope', 'us_sales_tax', 'none']
export function isTreatment(v: unknown): v is VatTreatment {
  return typeof v === 'string' && (TREATMENTS as string[]).includes(v)
}

type Notes = Partial<Record<VatTreatment, string>>

// Legal wording per supplier country: a mirror of api/internal/finance/tax
// (notes.go for the generic defaults, notes_de.go for Germany). Keep the text
// identical to the Go registry; the DE entries are bilingual "German / English".
const GENERIC_NOTES: Notes = {
  reverse_charge: 'Reverse charge: VAT to be accounted for by the recipient (Art. 196 Directive 2006/112/EC).',
  exempt: 'VAT exempt: small business scheme.',
  outside_scope: 'Outside the scope of EU VAT: place of supply outside the EU.',
}

const DE_NOTES: Notes = {
  exempt: 'Gemäß § 19 UStG wird keine Umsatzsteuer berechnet. / '
    + 'VAT is not charged under § 19 UStG (German small-business scheme).',
  reverse_charge: 'Steuerschuldnerschaft des Leistungsempfängers (Reverse Charge): '
    + 'Die Umsatzsteuer ist vom Leistungsempfänger zu entrichten (Art. 196 MwStSystRL). / '
    + 'Reverse charge: VAT to be accounted for by the recipient (Art. 196 Directive 2006/112/EC).',
  outside_scope: 'Nicht steuerbare sonstige Leistung, Leistungsort außerhalb Deutschlands (§ 3a Abs. 2 UStG). / '
    + 'Not subject to German VAT: the place of supply is outside Germany (§ 3a(2) UStG).',
}

/** The legal wording a supplier in `country` prints per treatment (those that carry one). */
export function notesFor(country: string): Notes {
  return country.trim().toUpperCase() === 'DE' ? { ...GENERIC_NOTES, ...DE_NOTES } : { ...GENERIC_NOTES }
}

export function party(p: Partial<Party>): Party {
  return {
    contact_id: null, legal_name: '', company: '', email: '', address_line1: '', address_line2: '',
    city: '', region: '', postal_code: '', country: '', vat_id: '', tax_id: '', is_business: true, ...p,
  }
}

// All names, addresses and tax IDs are fictional placeholders (example.com /
// example.test domains, zero-padded IDs). Never seed with real data.
export const DEFAULT_SUPPLIER: Supplier = {
  ...party({
    legal_name: 'Sam Example',
    company: 'Sample DJ Services',
    email: 'bookings@example.com',
    address_line1: '1 Example Street',
    city: 'Berlin',
    postal_code: '10000',
    country: 'DE',
    vat_id: 'DE000000000',
  }),
  vat_exempt_small_business: false,
  default_vat_rate_bps: 1900,
}

/** §3 tax suggestion for a customer, given the supplier's billing profile. */
export function suggest(supplier: Supplier, customer: Pick<Party, 'country' | 'vat_id' | 'is_business'>): TaxSuggestion {
  const sc = supplier.country.toUpperCase()
  const cc = (customer.country || '').toUpperCase()
  const notes = notesFor(sc)
  const out = (t: VatTreatment, rate: number, reason: string): TaxSuggestion => ({
    vat_treatment: t, tax_rate_bps: rate, tax_note: notes[t] ?? '', reason,
  })
  if (EU.has(sc)) {
    if (supplier.vat_exempt_small_business) return out('exempt', 0, 'You use the small-business VAT exemption.')
    if (!cc) return out('domestic', supplier.default_vat_rate_bps, 'customer country unknown — confirm')
    if (cc === sc) return out('domestic', supplier.default_vat_rate_bps, `Customer is in ${cc}, same country as you.`)
    if (EU.has(cc)) {
      if (!customer.vat_id) return out('domestic', supplier.default_vat_rate_bps, 'no customer VAT ID — confirm')
      if (!customer.is_business) return out('domestic', supplier.default_vat_rate_bps, 'customer not marked as a business — confirm')
      return out('reverse_charge', 0, `EU business customer in ${cc} with a VAT ID.`)
    }
    return out('outside_scope', 0, `Customer is in ${cc}, outside the EU.`)
  }
  if (sc === 'US') return out('none', 0, 'US supplier: no tax by default. Switch to US sales tax if it applies.')
  return out('none', 0, 'No rule for your country; no tax applied.')
}

/** VAT IDs are upper-cased with spaces and dots removed (same on contacts). */
export function normalizeVatId(v: unknown): string {
  return String(v ?? '').toUpperCase().replace(/[\s.]/g, '')
}

export function normalizeParty(p: Partial<Party>, base: Party): Party {
  const merged = { ...base, ...p }
  return {
    ...merged,
    country: String(merged.country ?? '').trim().toUpperCase(),
    vat_id: normalizeVatId(merged.vat_id),
  }
}

export function prefixProblem(raw: unknown): string {
  const p = String(raw ?? '').trim().toUpperCase()
  if (p.length < 1 || p.length > 20) return 'number_prefix: must be 1 to 20 characters'
  if (!/^[A-Z0-9]+$/.test(p)) return 'number_prefix: may only contain letters and digits'
  if (p === 'CN') return 'number_prefix: CN is reserved for credit notes'
  return ''
}

/** YYYY-MM-DD only (supply_date). */
export function isDateOnly(v: unknown): v is string {
  return typeof v === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(v) && !Number.isNaN(Date.parse(`${v}T00:00:00Z`))
}

/** due_at accepts RFC 3339 or YYYY-MM-DD; returns RFC 3339, null for empty, undefined when invalid. */
export function normalizeDueAt(v: unknown): string | null | undefined {
  if (v === null || v === undefined || v === '') return null
  if (typeof v !== 'string') return undefined
  if (isDateOnly(v)) return `${v}T00:00:00Z`
  return Number.isNaN(Date.parse(v)) ? undefined : v
}

/** RFC 4122 v4 id; falls back when crypto.randomUUID is unavailable. */
export function uuid(): string {
  const c = globalThis.crypto
  if (c && typeof c.randomUUID === 'function') return c.randomUUID()
  const b = new Uint8Array(16)
  if (c && typeof c.getRandomValues === 'function') c.getRandomValues(b)
  else for (let i = 0; i < 16; i++) b[i] = Math.floor(Math.random() * 256)
  b[6] = (b[6]! & 0x0f) | 0x40
  b[8] = (b[8]! & 0x3f) | 0x80
  const h = [...b].map((x) => x.toString(16).padStart(2, '0')).join('')
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`
}

/** Adds whole days to a YYYY-MM-DD date. */
export function addDays(date: string, days: number): string {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}

// ── EN 16931 data model (billing profile bank details, invoice lines) ──

/** IBAN length per country, as in api/internal/finance/validation.go. */
const IBAN_LENGTHS: Record<string, number> = {
  AD: 24, AE: 23, AL: 28, AT: 20, AZ: 28, BA: 20, BE: 16, BG: 22, BH: 22, BR: 29,
  CH: 21, CY: 28, CZ: 24, DE: 22, DK: 18, EE: 20, ES: 24, FI: 18, FR: 27, GB: 22,
  GE: 22, GI: 23, GR: 27, HR: 21, HU: 28, IE: 22, IL: 23, IS: 26, IT: 27, JO: 30,
  KW: 30, LB: 28, LI: 21, LT: 20, LU: 20, LV: 21, MC: 27, MD: 24, ME: 22, MK: 19,
  MT: 31, NL: 18, NO: 15, PL: 28, PT: 25, QA: 29, RO: 24, RS: 22, SA: 24, SE: 24,
  SI: 19, SK: 24, SM: 27, TR: 26, UA: 29, VA: 22, XK: 20,
}

/** Removes spaces and tabs and upper-cases (IBAN and BIC storage form). */
export function compactUpper(v: unknown): string {
  return String(v ?? '').trim().replace(/[ \t]/g, '').toUpperCase()
}

/** Known country, right length, ISO 7064 mod-97 check; `s` must already be compact + upper-case. */
export function validIban(s: string): boolean {
  if (s.length < 5 || IBAN_LENGTHS[s.slice(0, 2)] !== s.length) return false
  let rem = 0
  for (const ch of s.slice(4) + s.slice(0, 4)) {
    if (ch >= '0' && ch <= '9') rem = (rem * 10 + Number(ch)) % 97
    else if (ch >= 'A' && ch <= 'Z') rem = (rem * 100 + (ch.charCodeAt(0) - 55)) % 97
    else return false
  }
  return rem === 1
}

export const BIC_PATTERN = /^[A-Z]{4}[A-Z]{2}[A-Z0-9]{2}([A-Z0-9]{3})?$/

/** UN/ECE Rec. 20 units allowed on invoice lines. */
export const UNIT_CODES = ['C62', 'HUR', 'DAY', 'LS', 'KMT']

// Fictional billing profile (the supplier above, as the profile endpoint shows it).
export const DEFAULT_BILLING_PROFILE: BillingProfile = {
  id: '00000000-0000-4000-8000-0000000000b1',
  legal_name: 'Sam Example',
  trading_name: 'Sample DJ Services',
  entity_kind: 'sole_trader',
  tax_id: 'DE000000000',
  tax_id_kind: 'vat',
  contact_email: 'bookings@example.com',
  contact_phone: '',
  address_line1: '1 Example Street',
  address_line2: '',
  address_city: 'Berlin',
  address_region: '',
  address_postal: '10000',
  address_country: 'DE',
  jurisdiction: 'DE',
  payment_instructions: '',
  default_currency: 'EUR',
  tax_number: '',
  iban: '',
  bic: '',
  vat_exempt_small_business: false,
  default_vat_rate_bps: 1900,
  updated_at: '2026-01-01T00:00:00Z',
  created_at: '2026-01-01T00:00:00Z',
}
