// Pure invoicing rules (docs/INVOICING.md §3–§4.1) shared by the Nitro dev
// mocks (server/api/v1/finance) and the browser demo (app/demo). No I/O.

import type { Party, TaxSuggestion, VatTreatment } from '../../app/types/finance'

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

export const NOTES: Partial<Record<VatTreatment, string>> = {
  reverse_charge: 'Reverse charge: VAT to be accounted for by the recipient (Art. 196 Directive 2006/112/EC).',
  exempt: 'VAT exempt: small business scheme.',
  outside_scope: 'Outside the scope of EU VAT: place of supply outside the EU.',
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
  const out = (t: VatTreatment, rate: number, reason: string): TaxSuggestion => ({
    vat_treatment: t, tax_rate_bps: rate, tax_note: NOTES[t] ?? '', reason,
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
