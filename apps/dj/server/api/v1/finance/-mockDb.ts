// In-memory implementation of the invoicing contract (docs/INVOICING.md)
// for frontend-only dev. The leading "-" keeps Nuxt/Nitro from treating
// this file as a route; it lives under server/api/v1 so production builds
// drop it together with the other mocks (scripts/build-prod.mjs).

import type { H3Event } from 'h3'
import type {
  Invoice,
  InvoiceLine,
  IssueProblem,
  Party,
  Payment,
  TaxSuggestion,
  VatTreatment,
} from '../../../../app/types/finance'
import { parseMoney } from '../../../../app/utils/money'

// ── helpers ─────────────────────────────────────────────────────────────

let clock = Date.parse('2026-09-25T09:00:00Z')
/** Strictly increasing timestamps so every write gets a new updated_at. */
export function stamp(): string {
  clock = Math.max(clock + 1, Date.now())
  return new Date(clock).toISOString()
}

export function fail(event: H3Event, status: number, error: string, message: string, problems?: IssueProblem[]) {
  setResponseStatus(event, status)
  return problems ? { error, message, problems } : { error, message }
}

function applyBps(minor: number, bps: number): number {
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

// ── supplier (billing profile) ──────────────────────────────────────────

// All names, addresses and tax IDs below are fictional placeholders
// (example.com / example.test domains, zero-padded IDs). Never seed mocks
// with real venues, promoters or registration numbers.
export const supplier = {
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

// ── gigs known to the mock (the gig mocks proxy to the Go API) ─────────

interface MockGig { id: string; date: string; label: string; fee_minor: number; currency: string; customer: Party }

export const GIG_IDS = {
  domestic: '00000000-0000-4000-8000-000000000101',
  reverseCharge: '00000000-0000-4000-8000-000000000102',
  us: '00000000-0000-4000-8000-000000000103',
  incomplete: '00000000-0000-4000-8000-000000000104',
  credited: '00000000-0000-4000-8000-000000000105',
  unbilled: '00000000-0000-4000-8000-000000000106',
} as const

export const gigs = new Map<string, MockGig>([
  [GIG_IDS.domestic, { id: GIG_IDS.domestic, date: '2026-08-08', label: 'Club Alpha — Resident Night', fee_minor: 80000, currency: 'EUR',
    customer: party({ legal_name: 'Alpha Events GmbH', company: 'Club Alpha', email: 'office@alpha.example', address_line1: '10 Sample Road', city: 'Berlin', postal_code: '10001', country: 'DE', vat_id: 'DE000000001' }) }],
  [GIG_IDS.reverseCharge, { id: GIG_IDS.reverseCharge, date: '2026-08-22', label: 'Club Beta — Techno Night', fee_minor: 150000, currency: 'EUR',
    customer: party({ legal_name: 'Beta Nights SAS', company: 'Club Beta', email: 'booking@beta.example', address_line1: '20 Rue Exemple', city: 'Paris', postal_code: '75000', country: 'FR', vat_id: 'FR00000000000' }) }],
  [GIG_IDS.us, { id: GIG_IDS.us, date: '2026-09-12', label: 'Gamma Hall — Sunday Party', fee_minor: 250000, currency: 'USD',
    customer: party({ legal_name: 'Gamma Hall LLC', company: 'Gamma Hall', email: 'pay@gamma.example', address_line1: '30 Example Ave', city: 'New York', region: 'NY', postal_code: '10000', country: 'US', tax_id: '00-0000000' }) }],
  [GIG_IDS.incomplete, { id: GIG_IDS.incomplete, date: '2026-09-19', label: 'Delta Room — Room One', fee_minor: 120000, currency: 'EUR',
    customer: party({ legal_name: 'Delta Promotions', email: 'accounts@delta.example', country: 'GB' }) }],
  [GIG_IDS.credited, { id: GIG_IDS.credited, date: '2026-07-31', label: 'Epsilon Festival', fee_minor: 180000, currency: 'EUR',
    customer: party({ legal_name: 'Epsilon Festival BV', company: 'Epsilon Festival', address_line1: '40 Voorbeeldstraat', city: 'Amsterdam', postal_code: '1000', country: 'NL', vat_id: 'NL000000000B01' }) }],
  [GIG_IDS.unbilled, { id: GIG_IDS.unbilled, date: '2026-09-20', label: 'Zeta Garden — Open Air', fee_minor: 60000, currency: 'EUR',
    customer: party({ legal_name: 'Zeta Collective', address_line1: '50 Sample Lane', city: 'Berlin', postal_code: '10002', country: 'DE' }) }],
])

/**
 * Learns a real gig (fee, currency, date, promoter) through the gig route,
 * which proxies to the Go API when one is running. Without it the gig
 * stays unknown and gigFor() synthesizes placeholder fee info.
 */
export async function learnGig(id: string): Promise<void> {
  if (gigs.has(id)) return
  try {
    const res = await $fetch<unknown>(`/api/v1/gigs/${id}`)
    const g = ((res && typeof res === 'object' && 'data' in res ? (res as { data: unknown }).data : res) ?? {}) as Record<string, unknown>
    if (!g.id) return
    const currency = String(g.fee_currency || 'EUR')
    gigs.set(id, {
      id,
      date: String(g.date ?? '').slice(0, 10) || new Date().toISOString().slice(0, 10),
      label: String(g.event_name || g.venue || 'Gig'),
      fee_minor: parseMoney(String(g.fee_amount ?? 0), currency) ?? 0,
      currency,
      customer: party({ legal_name: String(g.promoter_name ?? ''), email: String(g.promoter_email ?? '') }),
    })
  } catch {
    // No backend (or no such gig): fall back to gigFor()'s placeholder.
  }
}

/** Unknown gig ids get synthesized fee info so any gig can be billed. */
export function gigFor(id: string): MockGig {
  return gigs.get(id) ?? {
    id, date: new Date().toISOString().slice(0, 10), label: 'Gig', fee_minor: 100000, currency: 'EUR', customer: party({}),
  }
}

// ── storage ─────────────────────────────────────────────────────────────

type StoredInvoice = Omit<Invoice, 'received_minor' | 'pending_minor' | 'outstanding_minor'>

export const invoices = new Map<string, StoredInvoice>()
export const lines = new Map<string, InvoiceLine[]>()
export const payments = new Map<string, Payment>()
const series = new Map<string, number>()

export function recompute(inv: StoredInvoice): void {
  const subtotal = (lines.get(inv.id) ?? []).reduce((s, l) => s + l.line_total_minor, 0)
  for (const l of lines.get(inv.id) ?? []) l.tax_bps = inv.tax_rate_bps
  inv.subtotal_minor = subtotal
  inv.tax_minor = applyBps(subtotal, inv.tax_rate_bps)
  inv.total_minor = subtotal + inv.tax_minor
  inv.withholding_minor = applyBps(subtotal, inv.withholding_rate_bps)
  inv.net_payable_minor = inv.total_minor - inv.withholding_minor
  inv.tax_breakdown = subtotal > 0 ? [{ rate_bps: inv.tax_rate_bps, taxable_minor: subtotal, tax_minor: inv.tax_minor }] : []
}

export function balances(inv: StoredInvoice) {
  if (inv.kind !== 'invoice' || inv.status === 'draft' || inv.status === 'cancelled') {
    return { received_minor: 0, pending_minor: 0, outstanding_minor: 0, pending_refunds: 0 }
  }
  let received = 0
  let pending = 0
  let pendingRefunds = 0
  for (const p of payments.values()) {
    if (p.invoice_id !== inv.id) continue
    const inMoney = p.kind !== 'refund'
    if (p.status === 'completed') received += inMoney ? p.amount_minor : -p.amount_minor
    else if (p.status === 'pending') {
      if (inMoney) pending += p.amount_minor
      else pendingRefunds += p.amount_minor
    }
  }
  return {
    received_minor: received,
    pending_minor: pending,
    // Only issued invoices owe money; paid/credited/corrected/cancelled owe nothing (§4.1).
    outstanding_minor: inv.status === 'issued' ? Math.max(0, inv.net_payable_minor - received - pending) : 0,
    pending_refunds: pendingRefunds,
  }
}

export function serialize(inv: StoredInvoice): Invoice {
  const { pending_refunds: _unused, ...b } = balances(inv)
  return { ...structuredClone(inv), ...b }
}

export function allocateNumber(prefix: string, currency: string): { number: string; seq: number } {
  const key = `${prefix.toUpperCase()}|${currency}`
  const seq = (series.get(key) ?? 0) + 1
  series.set(key, seq)
  return { number: `${prefix.toUpperCase()}-${String(seq).padStart(4, '0')}-${currency}`, seq }
}

// ── tax rules (§3) ──────────────────────────────────────────────────────

export function suggest(customer: Pick<Party, 'country' | 'vat_id' | 'is_business'>): TaxSuggestion {
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

export function issueProblems(inv: StoredInvoice): IssueProblem[] {
  const p: IssueProblem[] = []
  const need = (ok: boolean, field: string, message: string) => { if (!ok) p.push({ field, message }) }
  for (const f of ['legal_name', 'address_line1', 'city', 'postal_code', 'country'] as const) {
    need(!!supplier[f], `supplier.${f}`, `Your billing profile needs ${f.replace(/_/g, ' ')}.`)
  }
  const c = inv.customer
  need(!!(c.legal_name || c.company), 'customer.legal_name', 'Customer legal name or company is required.')
  need(!!c.address_line1, 'customer.address_line1', 'Customer address is required.')
  need(!!c.city, 'customer.city', 'Customer city is required.')
  need(!!c.country, 'customer.country', 'Customer country is required.')
  need(!!inv.supply_date, 'supply_date', 'Supply date is required.')
  need(inv.total_minor > 0, 'total_minor', 'The invoice total must be above zero.')
  const sEU = EU.has(supplier.country)
  const cEU = EU.has(c.country)
  switch (inv.vat_treatment) {
    case 'domestic':
      need(inv.tax_rate_bps > 0, 'tax_rate_bps', 'Domestic VAT needs a rate above 0%.')
      need(!sEU || !!supplier.vat_id, 'supplier.vat_id', 'Your billing profile needs a VAT ID.')
      break
    case 'reverse_charge':
      need(!!supplier.vat_id, 'supplier.vat_id', 'Your billing profile needs a VAT ID.')
      need(!!c.vat_id, 'customer.vat_id', 'Reverse charge needs the customer VAT ID.')
      need(sEU && cEU, 'customer.country', 'Reverse charge needs both parties in the EU.')
      need(!c.country || c.country !== supplier.country, 'customer.country', 'Reverse charge needs the customer in another EU country.')
      need(inv.tax_rate_bps === 0, 'tax_rate_bps', 'Reverse charge must use a 0% rate.')
      break
    case 'exempt':
    case 'outside_scope':
    case 'none':
      need(inv.tax_rate_bps === 0, 'tax_rate_bps', 'This treatment must use a 0% rate.')
      if (inv.vat_treatment !== 'none') need(!!inv.tax_note.trim(), 'tax_note', 'A legal note is required for this treatment.')
      break
  }
  need(inv.withholding_rate_bps >= 0 && inv.withholding_rate_bps <= 5000, 'withholding_rate_bps', 'Withholding must be between 0% and 50%.')
  return p
}

// ── construction ────────────────────────────────────────────────────────

export function newDraft(gigId: string, over: Partial<StoredInvoice> = {}): StoredInvoice {
  const gig = gigFor(gigId)
  const s = suggest(gig.customer)
  const now = stamp()
  const due = new Date(`${gig.date}T00:00:00Z`)
  due.setUTCDate(due.getUTCDate() + 14)
  const inv: StoredInvoice = {
    id: crypto.randomUUID(),
    kind: 'invoice',
    gig_id: gigId,
    credits_invoice_id: null,
    replaced_by_invoice_id: null,
    invoice_number: null,
    number_prefix: 'INV',
    number_seq: null,
    currency: gig.currency,
    status: 'draft',
    supply_date: gig.date,
    issued_at: null,
    due_at: due.toISOString().replace('.000Z', 'Z'),
    paid_at: null,
    payment_ref: '',
    internal_notes: '',
    customer: structuredClone(gig.customer),
    billing_profile: null,
    vat_treatment: s.vat_treatment,
    tax_rate_bps: s.tax_rate_bps,
    tax_note: s.tax_note,
    subtotal_minor: 0, tax_minor: 0, total_minor: 0,
    withholding_rate_bps: 0, withholding_minor: 0, net_payable_minor: 0,
    tax_breakdown: [],
    updated_at: now,
    created_at: now,
    ...over,
  }
  invoices.set(inv.id, inv)
  lines.set(inv.id, [{
    id: crypto.randomUUID(), invoice_id: inv.id, sort_order: 0,
    description: `DJ performance — ${gig.label} (${gig.date})`,
    quantity: 1, unit_minor: gig.fee_minor, tax_bps: inv.tax_rate_bps, line_total_minor: gig.fee_minor, created_at: now,
  }])
  recompute(inv)
  return inv
}

export function issue(inv: StoredInvoice, at = stamp()): void {
  const prefix = inv.kind === 'credit_note' ? 'CN' : inv.number_prefix || 'INV'
  const { number, seq } = allocateNumber(prefix, inv.currency)
  inv.invoice_number = number
  inv.number_seq = seq
  inv.status = 'issued'
  inv.issued_at = at
  inv.billing_profile = structuredClone(supplier) as unknown as Record<string, unknown>
  inv.updated_at = stamp()
}

export function creditNoteFor(orig: StoredInvoice, reason: string): StoredInvoice {
  const cn = newDraft(orig.gig_id, {
    kind: 'credit_note',
    credits_invoice_id: orig.id,
    currency: orig.currency,
    customer: structuredClone(orig.customer),
    vat_treatment: orig.vat_treatment,
    tax_rate_bps: orig.tax_rate_bps,
    tax_note: orig.tax_note,
    withholding_rate_bps: orig.withholding_rate_bps,
    supply_date: orig.supply_date,
    due_at: null,
    internal_notes: reason,
  })
  lines.set(cn.id, (lines.get(orig.id) ?? []).map((l) => ({ ...l, id: crypto.randomUUID(), invoice_id: cn.id })))
  recompute(cn)
  issue(cn)
  return cn
}

export function addPayment(invoiceId: string, p: Partial<Payment>): Payment {
  const now = stamp()
  const pay: Payment = {
    id: crypto.randomUUID(), invoice_id: invoiceId, currency: 'EUR', amount_minor: 0, kind: 'payment',
    status: 'pending', method: '', reference: '', received_at: null, created_at: now, updated_at: now, ...p,
  }
  payments.set(pay.id, pay)
  return pay
}

// ── validation / normalisation (§4.1) ───────────────────────────────────

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
export function isDateOnly(v: unknown): boolean {
  return typeof v === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(v) && !Number.isNaN(Date.parse(`${v}T00:00:00Z`))
}

/** due_at accepts RFC 3339 or YYYY-MM-DD; returns RFC 3339, null for empty, undefined when invalid. */
export function normalizeDueAt(v: unknown): string | null | undefined {
  if (v === null || v === undefined || v === '') return null
  if (typeof v !== 'string') return undefined
  if (isDateOnly(v)) return `${v}T00:00:00Z`
  return Number.isNaN(Date.parse(v)) ? undefined : v
}

export function badRequest(event: H3Event, errs: string[]) {
  return fail(event, 400, 'validation_failed', 'validation failed: ' + errs.join('; '))
}

// ── request helpers ─────────────────────────────────────────────────────

export function findInvoice(event: H3Event): StoredInvoice | undefined {
  return invoices.get(getRouterParam(event, 'id') ?? '')
}

/** Mirrors the Go handlers: missing token → 400, stale token → 409. */
export function checkToken(event: H3Event, inv: { updated_at: string }, token: unknown) {
  if (typeof token !== 'string' || token === '') return fail(event, 400, 'bad_request', 'updated_at is required')
  if (token !== inv.updated_at) return fail(event, 409, 'conflict', 'invoice updated by another writer')
  return null
}

// ── seed ────────────────────────────────────────────────────────────────

function seed(): void {
  // Paid domestic invoice (DE → DE, 19%).
  const paid = newDraft(GIG_IDS.domestic)
  issue(paid, '2026-08-09T10:00:00Z')
  addPayment(paid.id, { currency: 'EUR', amount_minor: paid.net_payable_minor, kind: 'payment', status: 'completed', method: 'bank transfer', received_at: '2026-08-20T00:00:00Z' })
  paid.status = 'paid'
  paid.paid_at = '2026-08-20T00:00:00Z'

  // Overdue EU reverse charge (DE → FR) with a received deposit.
  const rc = newDraft(GIG_IDS.reverseCharge, { due_at: '2026-09-05T00:00:00Z' })
  issue(rc, '2026-08-23T10:00:00Z')
  addPayment(rc.id, { currency: 'EUR', amount_minor: 50000, kind: 'deposit', status: 'completed', method: 'bank transfer', received_at: '2026-08-01T00:00:00Z' })

  // US customer, no tax, 30% withholding, a pending payment.
  const us = newDraft(GIG_IDS.us, {
    vat_treatment: 'none', tax_rate_bps: 0, tax_note: '', withholding_rate_bps: 3000, due_at: '2026-10-12T00:00:00Z',
  })
  recompute(us)
  issue(us, '2026-09-13T10:00:00Z')
  addPayment(us.id, { currency: 'USD', amount_minor: 100000, kind: 'payment', status: 'pending', method: 'wire' })

  // Draft with an incomplete customer (issue-check problems).
  newDraft(GIG_IDS.incomplete)

  // Credited invoice and its credit note.
  const credited = newDraft(GIG_IDS.credited)
  issue(credited, '2026-08-01T10:00:00Z')
  const cn = creditNoteFor(credited, 'Festival cancelled the slot')
  credited.status = 'credited'
  credited.updated_at = stamp()
  void cn
}

seed()
