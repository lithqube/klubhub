// Finance endpoints as pure functions over a FinanceMockDb. Each returns the
// status + JSON body of the Go API (docs/INVOICING.md §4), so the Nitro dev
// routes and the browser demo router answer identically.

import type { InvoiceSummary, Party, PaymentKind, PaymentRecordStatus } from '../../app/types/finance'
import type { FinanceMockDb, StoredInvoice } from './db'
import {
  badRequest, fail, isDateOnly, isTreatment, normalizeDueAt, normalizeParty, NOTES, ok, prefixProblem,
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
  return ok({ data: db.serialize(inv), lines: db.lines.get(inv.id) ?? [] })
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

/** Full replacement of the listed fields (§4.1); draft only. */
export function updateInvoice(db: FinanceMockDb, id: string, body: Body): MockResult {
  const inv = writable(db, id, body)
  if (isResult(inv)) return inv
  if (inv.status !== 'draft') return fail(409, 'bad_state', 'only draft invoices can be edited')

  const errs: string[] = []
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
