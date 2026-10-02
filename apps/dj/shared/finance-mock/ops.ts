// Finance endpoints as pure functions over a FinanceMockDb. Each returns the
// status + JSON body of the Go API (docs/INVOICING.md §4), so the Nitro dev
// routes and the browser demo router answer identically.

import type {
  BillingProfile,
  EntryFilter,
  EntryKind,
  EntryStatus,
  EntryTotals,
  InvoiceLine,
  InvoiceSummary,
  Party,
  PaymentKind,
  PaymentRecordStatus,
  ProfitLossTotals,
  ReconciliationAction,
} from '../../app/types/finance'
import { minorToDecimalString } from '../../app/utils/money'
import type { FinanceMockDb, StoredEntry, StoredInvoice } from './db'
import {
  badRequest, BIC_PATTERN, compactUpper, fail, isDateOnly, isTreatment, normalizeDueAt, normalizeParty, NOTES, ok,
  prefixProblem, UNIT_CODES, uuid, validIban,
  type MockResult,
} from './rules'

type Body = Record<string, unknown>
type Query = Record<string, unknown>

const STATUSES = ['draft', 'issued', 'paid', 'cancelled', 'credited', 'corrected']
const KINDS = ['invoice', 'credit_note']
const PAYMENT_KINDS: PaymentKind[] = ['deposit', 'payment', 'refund']
const PAYMENT_STATUSES: PaymentRecordStatus[] = ['pending', 'completed', 'failed', 'refunded']
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

const notFound = () => fail(404, 'not_found', 'invoice not found')

/** Mirrors the Go handlers: missing token → 400, stale token → 409. */
function checkToken(inv: { updated_at: string }, token: unknown, what = 'invoice'): MockResult | null {
  if (typeof token !== 'string' || token === '') return fail(400, 'bad_request', 'updated_at is required')
  if (token !== inv.updated_at) return fail(409, 'conflict', `${what} updated by another writer`)
  return null
}

/** Finds the invoice and checks the concurrency token. */
function writable(db: FinanceMockDb, id: string, body: Body): StoredInvoice | MockResult {
  const inv = db.invoices.get(id)
  if (!inv) return notFound()
  return checkToken(inv, body.updated_at) ?? inv
}
const isResult = (v: unknown): v is MockResult => !!v && typeof v === 'object' && 'status' in v && 'body' in v

export function listInvoices(db: FinanceMockDb, q: Query): MockResult {
  const errs: string[] = []
  if (q.status && !STATUSES.includes(String(q.status))) errs.push('status: unknown status')
  if (q.kind && !KINDS.includes(String(q.kind))) errs.push('kind: unknown kind')
  if (q.gig_id && !UUID.test(String(q.gig_id))) errs.push('gig_id: must be a UUID')
  if (errs.length) return badRequest(errs)
  const list = [...db.invoices.values()]
    .filter((i) => !q.status || i.status === q.status)
    .filter((i) => !q.gig_id || i.gig_id === q.gig_id)
    .filter((i) => !q.kind || i.kind === q.kind)
    .filter((i) => !q.currency || i.currency === String(q.currency).toUpperCase())
    .sort((a, b) => b.created_at.localeCompare(a.created_at))
    .slice(0, 100)
    .map((i) => db.serialize(i))
  return ok({ data: list })
}

export function getInvoice(db: FinanceMockDb, id: string): MockResult {
  const inv = db.invoices.get(id)
  if (!inv) return notFound()
  // Lines stored before unit codes existed read as pieces.
  const lines = (db.lines.get(inv.id) ?? []).map((l) => ({ ...l, unit_code: l.unit_code ?? 'C62' }))
  return ok({ data: db.serialize(inv), lines })
}

export function createInvoice(db: FinanceMockDb, body: Body | null): MockResult {
  if (!body || typeof body.gig_id !== 'string' || !body.gig_id) return badRequest(['gig_id: is required'])
  const gigId = body.gig_id
  const gig = db.gigFor(gigId)

  const errs: string[] = []
  const intIn = (v: unknown) => Number.isInteger(v) && (v as number) >= 0 && (v as number) <= 10000
  if (body.vat_treatment !== undefined && !isTreatment(body.vat_treatment)) errs.push('vat_treatment: unknown treatment')
  if (body.currency !== undefined && body.currency !== gig.currency) errs.push(`currency: must match the gig fee currency ${gig.currency}`)
  if (body.tax_rate_bps !== undefined && !intIn(body.tax_rate_bps)) errs.push('tax_rate_bps: must be between 0 and 10000')
  if (body.withholding_rate_bps !== undefined && !intIn(body.withholding_rate_bps)) errs.push('withholding_rate_bps: must be between 0 and 10000')
  if (body.supply_date !== undefined && body.supply_date !== null && !isDateOnly(body.supply_date)) errs.push('supply_date: must be a date in YYYY-MM-DD format')
  const due = normalizeDueAt(body.due_at)
  if (due === undefined) errs.push('due_at: must be RFC 3339 or YYYY-MM-DD')
  if (body.number_prefix !== undefined && String(body.number_prefix).trim() !== '') {
    const pp = prefixProblem(body.number_prefix)
    if (pp) errs.push(pp)
  }
  if (errs.length) return badRequest(errs)

  const active = [...db.invoices.values()].find(
    (i) => i.gig_id === gigId && i.kind === 'invoice' && ['draft', 'issued', 'paid'].includes(i.status),
  )
  if (active) return fail(409, 'bad_state', 'this gig already has an active invoice')

  const inv = db.newDraft(gigId)
  if (body.customer && typeof body.customer === 'object') inv.customer = normalizeParty(body.customer as Partial<Party>, inv.customer)
  if (isTreatment(body.vat_treatment)) {
    const suggested = db.suggest(inv.customer)
    if (body.vat_treatment !== suggested.vat_treatment) {
      // §4.1: a treatment other than the suggestion takes its default rate/note.
      inv.vat_treatment = body.vat_treatment
      inv.tax_rate_bps = body.vat_treatment === 'domestic' ? db.supplier.default_vat_rate_bps : 0
      inv.tax_note = NOTES[body.vat_treatment] ?? ''
    }
  }
  if (typeof body.tax_rate_bps === 'number') inv.tax_rate_bps = body.tax_rate_bps
  if (typeof body.withholding_rate_bps === 'number') inv.withholding_rate_bps = body.withholding_rate_bps
  if (typeof body.supply_date === 'string') inv.supply_date = body.supply_date
  if (due) inv.due_at = due
  if (typeof body.number_prefix === 'string' && body.number_prefix.trim()) inv.number_prefix = body.number_prefix.trim().toUpperCase()
  db.recompute(inv)
  return ok({ data: db.serialize(inv) }, 201)
}

// Limits of api/internal/finance/invoice_validation.go.
const MAX_LINES = 100
const MAX_LINE_DESCRIPTION = 500
const MAX_LINE_QUANTITY = 1_000_000
const MAX_LINE_TOTAL_MINOR = 10_000_000_000_000

/** Validates a `lines` replacement; returns the normalised lines or the field problems. */
function checkLines(raw: unknown): { lines: LineInput[] } | { errs: string[] } {
  if (!Array.isArray(raw) || raw.length === 0) return { errs: ['lines: an invoice needs at least one line'] }
  if (raw.length > MAX_LINES) return { errs: [`lines: at most ${MAX_LINES} lines are allowed`] }
  const errs: string[] = []
  const lines: LineInput[] = []
  raw.forEach((r, i) => {
    const l = (r && typeof r === 'object' ? r : {}) as Record<string, unknown>
    const path = `lines[${i}].`
    const description = String(l.description ?? '').trim()
    const n = [...description].length
    if (n === 0) errs.push(`${path}description: required`)
    else if (n > MAX_LINE_DESCRIPTION) errs.push(`${path}description: exceeds ${MAX_LINE_DESCRIPTION} characters`)
    const quantity = l.quantity as number
    if (!Number.isInteger(quantity) || quantity < 1 || quantity > MAX_LINE_QUANTITY) {
      errs.push(`${path}quantity: must be between 1 and ${MAX_LINE_QUANTITY}`)
    }
    const unit = l.unit_minor as number
    if (!Number.isInteger(unit)) errs.push(`${path}unit_minor: must be a whole number of minor units`)
    else if (unit < 0) errs.push(`${path}unit_minor: must not be negative`)
    else if (Number.isInteger(quantity) && quantity >= 1 && unit > 0 && quantity > Math.floor(MAX_LINE_TOTAL_MINOR / unit)) {
      errs.push(`${path}unit_minor: line total exceeds the maximum amount`)
    }
    const code = compactUpper(l.unit_code) || 'C62'
    if (!UNIT_CODES.includes(code)) errs.push(`${path}unit_code: must be one of ${UNIT_CODES.join(', ')}`)
    lines.push({ description, quantity, unit_minor: unit, unit_code: code })
  })
  return errs.length ? { errs } : { lines }
}

interface LineInput { description: string; quantity: number; unit_minor: number; unit_code: string }

/** Reference and terms fields of an invoice (EN 16931 BT-10/13/12/20) with their limits. */
const REFERENCE_LIMITS = [
  ['buyer_reference', 100], ['purchase_order_ref', 100], ['contract_ref', 100], ['payment_terms', 500],
] as const

/** Full replacement of the listed fields (§4.1); draft only. */
export function updateInvoice(db: FinanceMockDb, id: string, body: Body): MockResult {
  const inv = writable(db, id, body)
  if (isResult(inv)) return inv
  if (inv.status !== 'draft') return fail(409, 'bad_state', 'only draft invoices can be edited')

  const errs: string[] = []
  // Same order as the Go API: references, lines, then the rest.
  const refs: Record<string, string> = {}
  for (const [field, max] of REFERENCE_LIMITS) {
    refs[field] = String(body[field] ?? '').trim()
    if ([...refs[field]!].length > max) errs.push(`${field}: exceeds ${max} characters`)
  }
  // `lines` omitted/null keeps the existing lines; anything else replaces them.
  const linesChecked = body.lines === undefined || body.lines === null ? null : checkLines(body.lines)
  if (linesChecked && 'errs' in linesChecked) errs.push(...linesChecked.errs)
  const customer = normalizeParty((body.customer ?? {}) as Partial<Party>, inv.customer)
  if (customer.country && !/^[A-Z]{2}$/.test(customer.country)) errs.push('customer.country: must be a 2-letter ISO 3166-1 alpha-2 code')
  if (!isTreatment(body.vat_treatment)) errs.push('vat_treatment: must be one of domestic, reverse_charge, exempt, outside_scope, us_sales_tax, none')
  const rate = Number(body.tax_rate_bps)
  if (!Number.isInteger(rate) || rate < 0 || rate > 10000) errs.push('tax_rate_bps: must be between 0 and 10000')
  // Drafts accept 0–10000; issuing requires ≤ 5000 (issue-check).
  const wh = Number(body.withholding_rate_bps)
  if (!Number.isInteger(wh) || wh < 0 || wh > 10000) errs.push('withholding_rate_bps: must be between 0 and 10000')
  const supply = body.supply_date === '' || body.supply_date == null ? null : body.supply_date
  if (supply !== null && !isDateOnly(supply)) errs.push('supply_date: must be a date in YYYY-MM-DD format')
  const due = normalizeDueAt(body.due_at)
  if (due === undefined) errs.push('due_at: must be RFC 3339 or YYYY-MM-DD')
  const prefix = String(body.number_prefix ?? '').trim() || 'INV'
  const pp = prefixProblem(prefix)
  if (pp) errs.push(pp)
  if (errs.length) return badRequest(errs)

  inv.customer = customer
  inv.vat_treatment = body.vat_treatment as typeof inv.vat_treatment
  inv.tax_rate_bps = rate
  inv.tax_note = String(body.tax_note ?? '')
  inv.withholding_rate_bps = wh
  inv.supply_date = supply as string | null
  inv.due_at = due ?? null
  inv.number_prefix = prefix.toUpperCase()
  inv.internal_notes = String(body.internal_notes ?? '')
  inv.buyer_reference = refs.buyer_reference!
  inv.purchase_order_ref = refs.purchase_order_ref!
  inv.contract_ref = refs.contract_ref!
  inv.payment_terms = refs.payment_terms!
  if (linesChecked && 'lines' in linesChecked) {
    const now = db.stamp()
    db.lines.set(inv.id, linesChecked.lines.map((l, i): InvoiceLine => ({
      id: uuid(), invoice_id: inv.id, sort_order: i,
      description: l.description, quantity: l.quantity, unit_minor: l.unit_minor, unit_code: l.unit_code as InvoiceLine['unit_code'],
      // The server derives the tax rate and the line total; clients never send them.
      tax_bps: inv.tax_rate_bps, line_total_minor: l.quantity * l.unit_minor, created_at: now,
    })))
  }
  db.recompute(inv)
  inv.updated_at = db.stamp()
  return ok({ data: db.serialize(inv) })
}

export function issueCheck(db: FinanceMockDb, id: string): MockResult {
  const inv = db.invoices.get(id)
  if (!inv) return notFound()
  if (inv.status !== 'draft') return ok({ data: { ready: false, problems: [{ field: 'status', message: 'Only drafts can be issued.' }] } })
  const problems = db.issueProblems(inv)
  return ok({ data: { ready: problems.length === 0, problems } })
}

export function issueInvoice(db: FinanceMockDb, id: string, body: Body): MockResult {
  const inv = writable(db, id, body)
  if (isResult(inv)) return inv
  if (inv.status !== 'draft') return fail(409, 'bad_state', 'only drafts can be issued')
  const problems = db.issueProblems(inv)
  if (problems.length) return fail(422, 'not_issuable', 'invoice is not ready to issue', problems)
  db.issue(inv)
  return ok({ data: db.serialize(inv) })
}

export function cancelInvoice(db: FinanceMockDb, id: string, body: Body): MockResult {
  const inv = writable(db, id, body)
  if (isResult(inv)) return inv
  if (inv.status !== 'draft') return fail(409, 'bad_state', 'only drafts can be cancelled')
  inv.status = 'cancelled'
  inv.updated_at = db.stamp()
  return ok({ data: db.serialize(inv) })
}

export function payInvoice(db: FinanceMockDb, id: string, body: Body): MockResult {
  const inv = writable(db, id, body)
  if (isResult(inv)) return inv
  if (inv.kind !== 'invoice' || inv.status !== 'issued') return fail(409, 'bad_state', 'only issued invoices can be marked paid')
  inv.status = 'paid'
  inv.paid_at = typeof body.paid_at === 'string' && body.paid_at ? body.paid_at : db.stamp()
  inv.payment_ref = String(body.payment_ref ?? '')
  inv.updated_at = db.stamp()
  return ok({ data: db.serialize(inv) })
}

function creditable(db: FinanceMockDb, id: string, body: Body, verb: string): { inv: StoredInvoice; reason: string } | MockResult {
  const inv = writable(db, id, body)
  if (isResult(inv)) return inv
  if (inv.kind !== 'invoice' || (inv.status !== 'issued' && inv.status !== 'paid')) {
    return fail(409, 'bad_state', `only issued or paid invoices can be ${verb}`)
  }
  const reason = String(body.reason ?? '').trim()
  if (reason.length > 1000) return badRequest(['reason: exceeds 1000 characters'])
  return { inv, reason }
}

export function creditNote(db: FinanceMockDb, id: string, body: Body): MockResult {
  const r = creditable(db, id, body, 'credited')
  if (isResult(r)) return r
  const cn = db.creditNoteFor(r.inv, r.reason)
  r.inv.status = 'credited'
  r.inv.updated_at = db.stamp()
  return ok({ data: { credit_note: db.serialize(cn), original: db.serialize(r.inv) } }, 201)
}

export function correctInvoice(db: FinanceMockDb, id: string, body: Body): MockResult {
  const r = creditable(db, id, body, 'corrected')
  if (isResult(r)) return r
  const inv = r.inv
  const cn = db.creditNoteFor(inv, r.reason)
  const replacement = db.newDraft(inv.gig_id, {
    currency: inv.currency,
    customer: structuredClone(inv.customer),
    vat_treatment: inv.vat_treatment,
    tax_rate_bps: inv.tax_rate_bps,
    tax_note: inv.tax_note,
    withholding_rate_bps: inv.withholding_rate_bps,
    supply_date: inv.supply_date,
    due_at: inv.due_at,
    number_prefix: inv.number_prefix,
    internal_notes: `Replaces ${inv.invoice_number}`,
  })
  inv.status = 'corrected'
  inv.replaced_by_invoice_id = replacement.id
  inv.updated_at = db.stamp()
  return ok({ data: { credit_note: db.serialize(cn), original: db.serialize(inv), replacement: db.serialize(replacement) } }, 201)
}

export function listPayments(db: FinanceMockDb, id: string): MockResult {
  const inv = db.invoices.get(id)
  if (!inv) return notFound()
  const list = [...db.payments.values()]
    .filter((p) => p.invoice_id === inv.id)
    .sort((a, b) => a.created_at.localeCompare(b.created_at))
  return ok({ data: list })
}

export function createPayment(db: FinanceMockDb, id: string, body: Body): MockResult {
  const inv = db.invoices.get(id)
  if (!inv) return notFound()
  const amount = Number(body.amount_minor)
  const kind = body.kind as PaymentKind
  const errs: string[] = []
  if (!Number.isInteger(amount) || amount <= 0) errs.push('amount_minor: must be positive')
  if (!PAYMENT_KINDS.includes(kind)) errs.push('kind: must be one of deposit, payment, refund')
  if (errs.length) return badRequest(errs)
  if (inv.kind !== 'invoice' || (inv.status !== 'issued' && inv.status !== 'paid')) {
    return fail(422, 'invoice_not_payable', 'invoice does not accept payments in its current status')
  }
  if (body.currency !== inv.currency) return badRequest([`currency: must match invoice currency ${inv.currency}`])
  const b = db.balances(inv)
  const limit = kind === 'refund' ? b.received_minor - b.pending_refunds : b.outstanding_minor
  if (amount > limit) return fail(422, 'exceeds_balance', 'payment exceeds invoice balance')
  const p = db.addPayment(inv.id, {
    currency: inv.currency,
    amount_minor: amount,
    kind,
    method: String(body.method ?? ''),
    reference: String(body.reference ?? ''),
    received_at: typeof body.received_at === 'string' ? body.received_at : null,
  })
  return ok({ data: p }, 201)
}

export function getPayment(db: FinanceMockDb, id: string): MockResult {
  const p = db.payments.get(id)
  return p ? ok({ data: p }) : fail(404, 'not_found', 'payment not found')
}

export function updatePayment(db: FinanceMockDb, id: string, body: Body): MockResult {
  const p = db.payments.get(id)
  if (!p) return fail(404, 'not_found', 'payment not found')
  const tokenErr = checkToken(p, body.updated_at, 'payment')
  if (tokenErr) return tokenErr
  const status = body.status as PaymentRecordStatus
  if (!PAYMENT_STATUSES.includes(status)) {
    return badRequest(['status: must be one of pending, completed, failed, refunded'])
  }
  const inv = db.invoices.get(p.invoice_id)
  if (!inv || (inv.status !== 'issued' && inv.status !== 'paid')) {
    return fail(422, 'invoice_not_payable', 'invoice does not accept payments in its current status')
  }
  p.status = status
  p.method = String(body.method ?? p.method)
  p.reference = String(body.reference ?? p.reference)
  p.received_at = typeof body.received_at === 'string' ? body.received_at : p.received_at
  p.updated_at = db.stamp()
  return ok({ data: p })
}

/** §4.1: invoices only; paid_minor = net payable of paid + received (clamped) of issued. */
export function summaries(db: FinanceMockDb): MockResult {
  const out: Record<string, InvoiceSummary> = {}
  for (const stored of db.invoices.values()) {
    if (stored.kind !== 'invoice') continue
    const inv = db.serialize(stored)
    const s = (out[inv.currency] ??= {
      currency: inv.currency, draft_count: 0, issued_count: 0, paid_count: 0, outstanding_minor: 0, paid_minor: 0,
    })
    if (inv.status === 'draft') s.draft_count++
    if (inv.status === 'issued') {
      s.issued_count++
      s.outstanding_minor += inv.outstanding_minor
      s.paid_minor += Math.min(Math.max(inv.received_minor, 0), inv.net_payable_minor)
    }
    if (inv.status === 'paid') {
      s.paid_count++
      s.paid_minor += inv.net_payable_minor
    }
  }
  return ok({ data: out })
}

export function taxSuggestion(db: FinanceMockDb, q: Query): MockResult {
  return ok({
    data: db.suggest({
      country: String(q.customer_country ?? '').toUpperCase(),
      vat_id: String(q.customer_vat_id ?? ''),
      is_business: String(q.customer_is_business ?? 'false') === 'true',
    }),
  })
}

// ── Phase 5: earnings + reconciliations (FIN-01..FIN-09) ──────────────────

const ENTRY_KINDS: EntryKind[] = ['income', 'expense']
const ENTRY_STATUSES: EntryStatus[] = ['active', 'voided']
const RECONCILIATION_ACTIONS: ReconciliationAction[] = ['update', 'delete', 'void', 'keep']

const UUID_ANY = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

/** Strict token check (matches Go handler). */
function checkEntryToken(entry: StoredEntry, token: unknown): MockResult | null {
  if (typeof token !== 'string' || token === '') return fail(400, 'bad_request', 'updated_at is required')
  if (token !== entry.updated_at) return fail(409, 'conflict', 'finance entry updated by another writer; refresh and retry')
  return null
}

function isCurrencyCode(raw: unknown): boolean {
  if (typeof raw !== 'string' || raw.length !== 3) return false
  for (let i = 0; i < raw.length; i++) {
    const c = raw.charCodeAt(i)
    if (c < 65 || c > 90) return false
  }
  return true
}

/** Mirrors ValidateCreateEntry from the Go service. */
function validateEntryFields(input: Record<string, unknown>): string[] {
  const errs: string[] = []
  if (!ENTRY_KINDS.includes(input.kind as EntryKind)) errs.push('kind: must be income or expense')
  const amount = Number(input.amount_minor)
  if (!Number.isInteger(amount) || amount <= 0) errs.push('amount_minor: must be greater than zero')
  if (!isCurrencyCode(input.currency)) errs.push('currency: must be an uppercase 3-letter code')
  if (!isDateOnly(input.entry_date)) errs.push('entry_date: must be YYYY-MM-DD')
  if (String(input.category ?? '').trim() === '') errs.push('category: is required')
  if (input.kind === 'income') {
    if (String(input.description ?? '').trim() === '') errs.push('description: is required for income')
  } else if (input.kind === 'expense') {
    if (String(input.notes ?? '').trim() === '') errs.push('notes: is required for expense')
  }
  if (input.gig_id != null && input.gig_id !== '' && !UUID_ANY.test(String(input.gig_id))) {
    errs.push('gig_id: must be a UUID')
  }
  return errs
}

/** Mirrors the auto-generation rules: only generated income is immutable. */
function ensureMutable(entry: StoredEntry): MockResult | null {
  if (entry.auto_generated) return fail(409, 'immutable', 'auto-generated finance entries cannot be edited or deleted; void them instead')
  if (entry.status === 'voided') return fail(409, 'inactive', 'voided finance entries cannot be edited or deleted')
  return null
}

function coerceFilter(q: Query): { filter: EntryFilter; errors: string[] } {
  const errors: string[] = []
  const filter: EntryFilter = {}
  const kindRaw = String(q.kind ?? '')
  const statusRaw = String(q.status ?? '')
  const currencyRaw = String(q.currency ?? '')
  const categoryRaw = String(q.category ?? '')
  const fromRaw = String(q.from ?? '')
  const toRaw = String(q.to ?? '')
  const gigRaw = String(q.gig_id ?? '')
  const receiptRaw = String(q.receipt ?? '')

  if (receiptRaw && receiptRaw !== 'missing' && receiptRaw !== 'present') errors.push('receipt: must be missing or present')
  if (kindRaw && !ENTRY_KINDS.includes(kindRaw as EntryKind)) errors.push('kind: must be income or expense')
  if (statusRaw && !ENTRY_STATUSES.includes(statusRaw as EntryStatus)) errors.push('status: must be active or voided')
  if (currencyRaw && !isCurrencyCode(currencyRaw)) errors.push('currency: must be an uppercase 3-letter code')
  // categoryRaw is a presence-only filter; nothing to validate beyond a real string.
  void categoryRaw
  if (fromRaw && !isDateOnly(fromRaw)) errors.push('from: must be YYYY-MM-DD')
  if (toRaw && !isDateOnly(toRaw)) errors.push('to: must be YYYY-MM-DD')
  if (gigRaw && !UUID_ANY.test(gigRaw)) errors.push('gig_id: must be a UUID')

  if (kindRaw) filter.kind = kindRaw as EntryKind
  if (statusRaw) filter.status = statusRaw as EntryStatus
  if (currencyRaw) filter.currency = currencyRaw
  if (categoryRaw) filter.category = categoryRaw
  if (fromRaw) filter.from = fromRaw
  if (toRaw) filter.to = toRaw
  if (gigRaw) filter.gig_id = gigRaw
  if (receiptRaw === 'missing' || receiptRaw === 'present') filter.receipt = receiptRaw

  return { filter, errors }
}

function applyFilter(entries: StoredEntry[], f: EntryFilter, receiptCount: (id: string) => number = () => 0): StoredEntry[] {
  return entries.filter((e) => {
    // Same definition as the Go API: "missing" is an ACTIVE EXPENSE with no files.
    if (f.receipt === 'missing' && (e.kind !== 'expense' || e.status !== 'active' || receiptCount(e.id) > 0)) return false
    if (f.receipt === 'present' && receiptCount(e.id) === 0) return false
    if (f.kind && e.kind !== f.kind) return false
    if (f.status && e.status !== f.status) return false
    if (f.currency && e.currency !== f.currency) return false
    if (f.category && e.category !== f.category) return false
    if (f.gig_id && e.gig_id !== f.gig_id) return false
    if (f.from && e.entry_date < f.from) return false
    if (f.to && e.entry_date >= f.to) return false
    return true
  })
}

export function listEntries(db: FinanceMockDb, q: Query): MockResult {
  const { filter, errors } = coerceFilter(q)
  if (errors.length) return badRequest(errors)
  const rows = applyFilter([...db.entries.values()].filter((e) => !e.deleted_at), filter, (id) => db.attachmentsOf(id).length)
    .sort((a, b) => b.entry_date.localeCompare(a.entry_date))
    .slice(0, 200)
  return ok({ data: rows.map((e) => db.entryView(e)) })
}

export function getEntry(db: FinanceMockDb, id: string): MockResult {
  const e = db.entries.get(id)
  if (!e || e.deleted_at) return fail(404, 'not_found', 'finance entry not found')
  return ok({ data: db.entryView(e) })
}

/** Common body for POST /entries. */
export function createEntry(db: FinanceMockDb, body: Body | null): MockResult {
  const raw = (body ?? {}) as Record<string, unknown>
  const errs = validateEntryFields(raw)
  if (errs.length) return badRequest(errs)
  const input = {
    kind: raw.kind as EntryKind,
    amount_minor: Number(raw.amount_minor),
    currency: String(raw.currency),
    category: String(raw.category),
    entry_date: String(raw.entry_date),
    description: String(raw.description ?? ''),
    notes: String(raw.notes ?? ''),
    gig_id: raw.gig_id == null || raw.gig_id === '' ? null : String(raw.gig_id),
  }
  const e = db.addEntry({
    kind: input.kind,
    amount_minor: input.amount_minor,
    currency: input.currency,
    category: input.category,
    entry_date: input.entry_date,
    description: input.description,
    notes: input.notes,
    gig_id: input.gig_id,
  })
  return ok({ data: db.entryView(e) }, 201)
}

export function updateEntry(db: FinanceMockDb, id: string, body: Body): MockResult {
  const entry = db.entries.get(id)
  if (!entry || entry.deleted_at) return fail(404, 'not_found', 'finance entry not found')
  const mut = ensureMutable(entry)
  if (mut) return mut
  const token = checkEntryToken(entry, body.updated_at)
  if (token) return token
  const errs = validateEntryFields(body as unknown as Record<string, unknown>)
  if (errs.length) return badRequest(errs)
  entry.kind = body.kind as EntryKind
  entry.amount_minor = Number(body.amount_minor)
  entry.currency = String(body.currency)
  entry.category = String(body.category)
  entry.entry_date = String(body.entry_date)
  entry.description = String(body.description ?? '')
  entry.notes = String(body.notes ?? '')
  entry.gig_id = body.gig_id == null || body.gig_id === '' ? null : String(body.gig_id)
  entry.updated_at = db.stamp()
  return ok({ data: db.entryView(entry) })
}

export function deleteEntry(db: FinanceMockDb, id: string, body: Body): MockResult {
  const entry = db.entries.get(id)
  if (!entry || entry.deleted_at) return fail(404, 'not_found', 'finance entry not found')
  const mut = ensureMutable(entry)
  if (mut) return mut
  const token = checkEntryToken(entry, body.updated_at)
  if (token) return token
  entry.deleted_at = db.stamp()
  entry.updated_at = entry.deleted_at
  return { status: 204, body: {} }
}

export function voidEntry(db: FinanceMockDb, id: string, body: Body): MockResult {
  const entry = db.entries.get(id)
  if (!entry || entry.deleted_at) return fail(404, 'not_found', 'finance entry not found')
  const token = checkEntryToken(entry, body.updated_at)
  if (token) return token
  if (entry.status === 'voided') return fail(409, 'inactive', 'finance entry is already voided')
  entry.status = 'voided'
  entry.updated_at = db.stamp()
  return ok({ data: db.entryView(entry) })
}

/** Aggregates the active entries in `entries` (defaults to all) by currency. */
function aggregateTotals(entries: StoredEntry[]): Record<string, EntryTotals> {
  const out: Record<string, EntryTotals> = {}
  for (const e of entries) {
    if (e.deleted_at || e.status !== 'active') continue
    const row = (out[e.currency] ??= { currency: e.currency, income_minor: 0, expense_minor: 0 })
    if (e.kind === 'income') row.income_minor += e.amount_minor
    else row.expense_minor += e.amount_minor
  }
  return out
}

export function summaryEntries(db: FinanceMockDb, q: Query): MockResult {
  const scope = String(q.scope ?? '') as 'month' | 'year' | ''
  const year = Number(q.year ?? 0)
  const month = Number(q.month ?? 0)
  const fromRaw = String(q.from ?? '')
  const toRaw = String(q.to ?? '')

  let from = ''
  let to = ''
  if (fromRaw || toRaw) {
    if (!isDateOnly(fromRaw)) return badRequest(['from: must be YYYY-MM-DD'])
    if (!isDateOnly(toRaw)) return badRequest(['to: must be YYYY-MM-DD'])
    if (fromRaw >= toRaw) return badRequest(['to: must be after from (exclusive upper bound)'])
    from = fromRaw
    to = toRaw
  } else if (scope === 'month') {
    if (year < 1 || month < 1 || month > 12) return badRequest(['month: year and month are required'])
    const start = new Date(Date.UTC(year, month - 1, 1))
    const end = new Date(Date.UTC(year, month, 1))
    from = start.toISOString().slice(0, 10)
    to = end.toISOString().slice(0, 10)
  } else if (scope === 'year') {
    if (year < 1) return badRequest(['year: is required'])
    const start = new Date(Date.UTC(year, 0, 1))
    const end = new Date(Date.UTC(year + 1, 0, 1))
    from = start.toISOString().slice(0, 10)
    to = end.toISOString().slice(0, 10)
  } else {
    return fail(400, 'bad_request', 'scope (month|year) or from+to is required')
  }

  const filtered = [...db.entries.values()].filter((e) => e.entry_date >= from && e.entry_date < to)
  return ok({ data: aggregateTotals(filtered) as unknown as Record<string, EntryTotals> })
}

function profitLoss(entries: StoredEntry[]): Record<string, ProfitLossTotals> {
  const totals = aggregateTotals(entries)
  const out: Record<string, ProfitLossTotals> = {}
  for (const c of Object.keys(totals)) {
    const t = totals[c]!
    out[c] = {
      currency: t.currency,
      income_minor: t.income_minor,
      expense_minor: t.expense_minor,
      profit_loss_minor: t.income_minor - t.expense_minor,
    }
  }
  return out
}

export function profitLossScope(db: FinanceMockDb, q: Query): MockResult {
  const scope = String(q.scope ?? '') as 'gig' | 'month' | 'year' | ''
  const year = Number(q.year ?? 0)
  const month = Number(q.month ?? 0)
  const gigRaw = String(q.gig_id ?? '')

  // eslint-disable-next-line no-useless-assignment -- every branch reassigns before reading.
  let filtered: StoredEntry[] = []
  if (scope === 'gig') {
    if (!UUID_ANY.test(gigRaw)) return badRequest(['gig_id: must be a UUID'])
    filtered = [...db.entries.values()].filter((e) => e.gig_id === gigRaw)
  } else if (scope === 'month') {
    if (year < 1 || month < 1 || month > 12) return badRequest(['month: year and month are required'])
    const start = new Date(Date.UTC(year, month - 1, 1))
    const end = new Date(Date.UTC(year, month, 1))
    const from = start.toISOString().slice(0, 10)
    const to = end.toISOString().slice(0, 10)
    filtered = [...db.entries.values()].filter((e) => e.entry_date >= from && e.entry_date < to)
  } else if (scope === 'year') {
    if (year < 1) return badRequest(['year: is required'])
    const start = new Date(Date.UTC(year, 0, 1))
    const end = new Date(Date.UTC(year + 1, 0, 1))
    const from = start.toISOString().slice(0, 10)
    const to = end.toISOString().slice(0, 10)
    filtered = [...db.entries.values()].filter((e) => e.entry_date >= from && e.entry_date < to)
  } else {
    return fail(400, 'bad_request', 'scope (gig|month|year) is required')
  }
  return ok({ data: profitLoss(filtered) as unknown as Record<string, ProfitLossTotals> })
}

export function getReconciliationByGig(db: FinanceMockDb, q: Query): MockResult {
  const gigRaw = String(q.gig_id ?? '')
  if (!UUID_ANY.test(gigRaw)) return badRequest(['gig_id: must be a UUID'])
  for (const r of db.reconciliations.values()) {
    if (r.gig_id === gigRaw && r.status === 'pending') return ok({ data: r })
  }
  return fail(404, 'not_found', 'no pending reconciliation for gig')
}

export function resolveReconciliation(db: FinanceMockDb, id: string, body: Body): MockResult {
  const rec = db.reconciliations.get(id)
  if (!rec) return fail(404, 'not_found', 'reconciliation not found')
  if (rec.status !== 'pending') return fail(404, 'not_found', 'reconciliation already resolved')

  const tokenStr = body.updated_at
  if (typeof tokenStr !== 'string' || !tokenStr) return fail(400, 'bad_request', 'updated_at is required')
  if (tokenStr !== rec.updated_at) return fail(409, 'conflict', 'reconciliation changed; refresh and retry')

  const action = body.action as ReconciliationAction
  if (!RECONCILIATION_ACTIONS.includes(action)) {
    return badRequest(['action: must be update, delete, void, or keep'])
  }
  if (rec.allowed_actions.indexOf(action) === -1) {
    return fail(400, 'validation_failed', 'action is not allowed for this reconciliation')
  }

  const entry = db.entries.get(rec.entry_id)
  if (!entry) return fail(404, 'not_found', 'finance entry not found')

  if (action === 'update') {
    entry.amount_minor = rec.gig_amount_minor
    entry.currency = rec.gig_currency
    entry.source_amount_minor = rec.gig_amount_minor
    entry.source_currency = rec.gig_currency
    entry.updated_at = db.stamp()
  } else if (action === 'delete') {
    entry.deleted_at = db.stamp()
    entry.updated_at = entry.deleted_at
  } else if (action === 'void') {
    entry.status = 'voided'
    entry.updated_at = db.stamp()
  } else if (action === 'keep') {
    entry.source_amount_minor = rec.gig_amount_minor
    entry.source_currency = rec.gig_currency
    entry.updated_at = db.stamp()
  }

  rec.status = 'resolved'
  rec.resolution = action
  rec.updated_at = db.stamp()
  rec.resolved_at = rec.updated_at

  return ok({ data: rec })
}

/** Synthesises a reconcile-on-update payload for the gig route. */
export function describePendingMetadataForGig(db: FinanceMockDb, gigId: string): {
  metadata: unknown
} | null {
  for (const r of db.reconciliations.values()) {
    if (r.gig_id !== gigId || r.status !== 'pending') continue
    return {
      metadata: {
        id: r.id,
        entry_id: r.entry_id,
        reason: r.reason,
        allowed_actions: r.allowed_actions,
        updated_at: r.updated_at,
      },
    }
  }
  return null
}

// ── Billing profile (GET/PUT /api/v1/finance/billing-profile) ────────────

export function getBillingProfile(db: FinanceMockDb): MockResult {
  return ok({ data: structuredClone(db.billingProfile) })
}

const ENTITY_KINDS = ['individual', 'sole_trader', 'partnership', 'llc', 'corp', 'other']
const TAX_ID_KINDS = ['', 'vat', 'ein', 'gst', 'abn', 'other']

/** Same rules, messages and normalisation as api/internal/finance/validation.go. */
export function updateBillingProfile(db: FinanceMockDb, body: Body | null): MockResult {
  const b = body ?? {}
  if (typeof b.updated_at !== 'string' || !b.updated_at) return fail(400, 'bad_request', 'updated_at is required')
  const str = (k: string) => String(b[k] ?? '')
  const errs: string[] = []
  const required = (k: string, max = 0) => {
    const v = str(k).trim()
    if (!v) errs.push(`${k}: required`)
    else if (max && [...v].length > max) errs.push(`${k}: exceeds ${max} characters`)
  }
  required('legal_name', 200)
  const email = str('contact_email').trim()
  if (!email) errs.push('contact_email: required')
  else if (!/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(email)) errs.push('contact_email: not a valid email')
  required('address_line1')
  required('address_city')
  required('address_postal')
  required('jurisdiction')
  const currency = str('default_currency').trim()
  if (!currency) errs.push('default_currency: required')
  else if (currency.length !== 3) errs.push('default_currency: must be a 3-letter ISO 4217 code')
  else if (!/^[A-Z]{3}$/.test(currency)) errs.push('default_currency: must be uppercase A-Z letters')
  if (!ENTITY_KINDS.includes(str('entity_kind'))) errs.push('entity_kind: must be one of individual, sole_trader, partnership, llc, corp, other')
  if (!TAX_ID_KINDS.includes(str('tax_id_kind'))) errs.push("tax_id_kind: must be one of '', vat, ein, gst, abn, other")
  const country = str('address_country')
  if (country && country.length !== 2) errs.push('address_country: must be a 2-letter ISO 3166-1 alpha-2 code')
  else if (country && !/^[A-Z]{2}$/.test(country)) errs.push('address_country: must be uppercase A-Z letters')
  const rate = b.default_vat_rate_bps
  if (!Number.isInteger(rate) || (rate as number) < 0 || (rate as number) > 10000) errs.push('default_vat_rate_bps: must be between 0 and 10000 basis points')
  const taxNumber = str('tax_number').trim()
  const iban = compactUpper(b.iban)
  const bic = compactUpper(b.bic)
  if ([...taxNumber].length > 40) errs.push('tax_number: exceeds 40 characters')
  if (iban && !validIban(iban)) errs.push('iban: not a valid IBAN (check the country, length and check digits)')
  if (bic && !BIC_PATTERN.test(bic)) errs.push('bic: must be an 8 or 11 character BIC/SWIFT code')
  if ([...str('trading_name').trim()].length > 200) errs.push('trading_name: exceeds 200 characters')
  if (errs.length) return badRequest(errs)
  if (b.updated_at !== db.billingProfile.updated_at) return fail(409, 'conflict', 'billing profile updated by another writer; refresh and retry')

  db.billingProfile = {
    ...db.billingProfile,
    legal_name: str('legal_name').trim(), trading_name: str('trading_name').trim(),
    entity_kind: str('entity_kind') as BillingProfile['entity_kind'],
    tax_id: str('tax_id').trim(), tax_id_kind: str('tax_id_kind') as BillingProfile['tax_id_kind'],
    contact_email: email, contact_phone: str('contact_phone').trim(),
    address_line1: str('address_line1').trim(), address_line2: str('address_line2').trim(),
    address_city: str('address_city').trim(), address_region: str('address_region').trim(),
    address_postal: str('address_postal').trim(), address_country: country,
    jurisdiction: str('jurisdiction').trim(), payment_instructions: str('payment_instructions'),
    default_currency: currency, tax_number: taxNumber, iban, bic,
    vat_exempt_small_business: b.vat_exempt_small_business === true,
    default_vat_rate_bps: rate as number,
    updated_at: db.stamp(),
  }
  return ok({ data: structuredClone(db.billingProfile) })
}

// ── Invoice PDF (GET /api/v1/finance/invoices/{id}/pdf) ─────────────────

/** Text lines of a demo invoice PDF; null when the invoice does not exist. */
export function invoicePdfLines(db: FinanceMockDb, id: string): { name: string; lines: { text: string; size?: number; bold?: boolean }[] } | null {
  const inv = db.invoices.get(id)
  if (!inv) return null
  const money = (minor: number) => `${minorToDecimalString(minor, inv.currency)} ${inv.currency}`
  const label = inv.invoice_number ?? 'DRAFT'
  const out: { text: string; size?: number; bold?: boolean }[] = [
    { text: `${inv.kind === 'credit_note' ? 'Credit note' : 'Invoice'} ${label}`, size: 20, bold: true },
    { text: 'KlubHub DJ mock - fictional data, not a real invoice', size: 9 },
    { text: ' ' },
    { text: `From: ${db.supplier.legal_name}${db.billingProfile.iban ? ` - IBAN ${db.billingProfile.iban}` : ''}` },
    { text: `Bill to: ${inv.customer.company || inv.customer.legal_name || '-'}` },
    { text: `Supply date: ${inv.supply_date ?? '-'}` },
  ]
  for (const [lbl, v] of [
    ['Buyer reference', inv.buyer_reference], ['Purchase order', inv.purchase_order_ref],
    ['Contract', inv.contract_ref], ['Payment terms', inv.payment_terms],
  ] as const) if (v) out.push({ text: `${lbl}: ${v}` })
  out.push({ text: ' ' }, { text: 'Lines', bold: true })
  for (const l of db.lines.get(inv.id) ?? []) {
    out.push({ text: `${l.description} - ${l.quantity} x ${money(l.unit_minor)} [${l.unit_code ?? 'C62'}] = ${money(l.line_total_minor)}` })
  }
  out.push({ text: ' ' }, { text: `Subtotal: ${money(inv.subtotal_minor)}` }, { text: `Tax: ${money(inv.tax_minor)}` }, { text: `Total: ${money(inv.total_minor)}`, bold: true })
  return { name: `${label}.pdf`, lines: out }
}
