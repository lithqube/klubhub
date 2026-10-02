// In-memory invoice store implementing the invoicing contract
// (docs/INVOICING.md). One instance backs the Nitro dev mocks, another the
// browser demo; the demo persists it with toJSON()/load().

import type {
  BillingProfile,
  Entry,
  EntryAttachment,
  EntryKind,
  EntryReconciliation,
  EntrySource,
  Invoice,
  InvoiceLine,
  IssueProblem,
  Party,
  Payment,
  ReconciliationAction,
  ReconciliationReason,
  TaxSuggestion,
} from '../../app/types/finance'
import type { StoredAttachment } from './attachments'
import { applyBps, addDays, DEFAULT_BILLING_PROFILE, DEFAULT_SUPPLIER, EU, party, suggest, uuid, type Supplier } from './rules'

/** What the finance mock needs to know about a gig to bill it. */
export interface MockGig {
  id: string
  date: string
  label: string
  fee_minor: number
  currency: string
  customer: Party
}

export type StoredInvoice = Omit<Invoice, 'received_minor' | 'pending_minor' | 'outstanding_minor'>

export interface FinanceSnapshot {
  invoices: StoredInvoice[]
  lines: Record<string, InvoiceLine[]>
  payments: Payment[]
  series: Record<string, number>
  entries: StoredEntry[]
  reconciliations: StoredReconciliation[]
  seriesEntries: Record<string, number>
  /** Absent in snapshots saved before the billing profile endpoint existed. */
  billingProfile?: BillingProfile
  /** Receipt files (bytes as base64); absent before receipts existed. */
  attachments?: PersistedAttachment[]
  clock: number
}

/** A receipt as saved in the demo snapshot. */
export type PersistedAttachment = EntryAttachment & { data: string }

/** localStorage is small: receipts past this many bytes stay session-only. */
const PERSIST_ATTACHMENT_BUDGET = 1_000_000

function toBase64(bytes: Uint8Array): string {
  let bin = ''
  for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  return btoa(bin)
}

function fromBase64(data: string): Uint8Array {
  const bin = atob(data)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

/** Stored shape omits the server-computed fields (attachment_count) to keep load()/JSON clean. */
export type StoredEntry = Omit<Entry, 'attachment_count'>
export type StoredReconciliation = Omit<EntryReconciliation, never>

export interface FinanceMockOptions {
  /** Resolves a gig id; unknown ids get placeholder fee info. */
  lookupGig: (id: string) => MockGig | undefined
  supplier?: Supplier
  /** Start of the monotonic clock used for updated_at stamps. */
  clockStart?: number
}

export class FinanceMockDb {
  readonly invoices = new Map<string, StoredInvoice>()
  readonly lines = new Map<string, InvoiceLine[]>()
  readonly payments = new Map<string, Payment>()
  readonly series = new Map<string, number>()
  /** Phase 5: earnings ledger + durable finance reconciliations. */
  readonly entries = new Map<string, StoredEntry>()
  readonly reconciliations = new Map<string, StoredReconciliation>()
  readonly seriesEntries = new Map<string, number>()
  /** Receipt files by entry id, oldest first. Kept with an entry even when it is voided or deleted. */
  readonly attachments = new Map<string, StoredAttachment[]>()
  readonly supplier: Supplier
  /** The editable billing profile behind GET/PUT /finance/billing-profile. */
  billingProfile: BillingProfile = structuredClone(DEFAULT_BILLING_PROFILE)
  private clock: number
  private readonly lookupGig: FinanceMockOptions['lookupGig']

  constructor(opts: FinanceMockOptions) {
    this.lookupGig = opts.lookupGig
    this.supplier = opts.supplier ?? structuredClone(DEFAULT_SUPPLIER)
    this.clock = opts.clockStart ?? Date.now()
  }

  /** Strictly increasing timestamps so every write gets a new updated_at. */
  stamp(): string {
    this.clock = Math.max(this.clock + 1, Date.now())
    return new Date(this.clock).toISOString()
  }

  gigFor(id: string): MockGig {
    return this.lookupGig(id) ?? {
      id, date: new Date().toISOString().slice(0, 10), label: 'Gig', fee_minor: 100000, currency: 'EUR', customer: party({}),
    }
  }

  suggest(customer: Pick<Party, 'country' | 'vat_id' | 'is_business'>): TaxSuggestion {
    return suggest(this.supplier, customer)
  }

  recompute(inv: StoredInvoice): void {
    const ls = this.lines.get(inv.id) ?? []
    const subtotal = ls.reduce((s, l) => s + l.line_total_minor, 0)
    for (const l of ls) l.tax_bps = inv.tax_rate_bps
    inv.subtotal_minor = subtotal
    inv.tax_minor = applyBps(subtotal, inv.tax_rate_bps)
    inv.total_minor = subtotal + inv.tax_minor
    inv.withholding_minor = applyBps(subtotal, inv.withholding_rate_bps)
    inv.net_payable_minor = inv.total_minor - inv.withholding_minor
    inv.tax_breakdown = subtotal > 0 ? [{ rate_bps: inv.tax_rate_bps, taxable_minor: subtotal, tax_minor: inv.tax_minor }] : []
  }

  balances(inv: StoredInvoice) {
    if (inv.kind !== 'invoice' || inv.status === 'draft' || inv.status === 'cancelled') {
      return { received_minor: 0, pending_minor: 0, outstanding_minor: 0, pending_refunds: 0 }
    }
    let received = 0
    let pending = 0
    let pendingRefunds = 0
    for (const p of this.payments.values()) {
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

  serialize(inv: StoredInvoice): Invoice {
    const { pending_refunds: _unused, ...b } = this.balances(inv)
    const out = { ...structuredClone(inv), ...b }
    // Stored drafts from before the EN 16931 fields existed read as empty.
    out.buyer_reference ??= ''
    out.purchase_order_ref ??= ''
    out.contract_ref ??= ''
    out.payment_terms ??= ''
    return out
  }

  allocateNumber(prefix: string, currency: string): { number: string; seq: number } {
    const key = `${prefix.toUpperCase()}|${currency}`
    const seq = (this.series.get(key) ?? 0) + 1
    this.series.set(key, seq)
    return { number: `${prefix.toUpperCase()}-${String(seq).padStart(4, '0')}-${currency}`, seq }
  }

  issueProblems(inv: StoredInvoice): IssueProblem[] {
    const supplier = this.supplier
    const p: IssueProblem[] = []
    const need = (okay: boolean, field: string, message: string) => { if (!okay) p.push({ field, message }) }
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

  newDraft(gigId: string, over: Partial<StoredInvoice> = {}): StoredInvoice {
    const gig = this.gigFor(gigId)
    const s = this.suggest(gig.customer)
    const now = this.stamp()
    const inv: StoredInvoice = {
      id: uuid(),
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
      due_at: `${addDays(gig.date, 14)}T00:00:00Z`,
      paid_at: null,
      payment_ref: '',
      internal_notes: '',
      buyer_reference: '',
      purchase_order_ref: '',
      contract_ref: '',
      payment_terms: '',
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
    this.invoices.set(inv.id, inv)
    this.lines.set(inv.id, [{
      id: uuid(), invoice_id: inv.id, sort_order: 0,
      description: `DJ performance — ${gig.label} (${gig.date})`,
      quantity: 1, unit_minor: gig.fee_minor, unit_code: 'C62', tax_bps: inv.tax_rate_bps, line_total_minor: gig.fee_minor, created_at: now,
    }])
    this.recompute(inv)
    return inv
  }

  issue(inv: StoredInvoice, at = this.stamp()): void {
    const prefix = inv.kind === 'credit_note' ? 'CN' : inv.number_prefix || 'INV'
    const { number, seq } = this.allocateNumber(prefix, inv.currency)
    inv.invoice_number = number
    inv.number_seq = seq
    inv.status = 'issued'
    inv.issued_at = at
    inv.billing_profile = structuredClone(this.supplier) as unknown as Record<string, unknown>
    inv.updated_at = this.stamp()
  }

  creditNoteFor(orig: StoredInvoice, reason: string): StoredInvoice {
    const cn = this.newDraft(orig.gig_id, {
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
    this.lines.set(cn.id, (this.lines.get(orig.id) ?? []).map((l) => ({ ...l, id: uuid(), invoice_id: cn.id })))
    this.recompute(cn)
    this.issue(cn)
    return cn
  }

  addPayment(invoiceId: string, p: Partial<Payment>): Payment {
    const now = this.stamp()
    const pay: Payment = {
      id: uuid(), invoice_id: invoiceId, currency: 'EUR', amount_minor: 0, kind: 'payment',
      status: 'pending', method: '', reference: '', received_at: null, created_at: now, updated_at: now, ...p,
    }
    this.payments.set(pay.id, pay)
    return pay
  }

  toJSON(): FinanceSnapshot {
    return structuredClone({
      invoices: [...this.invoices.values()],
      lines: Object.fromEntries(this.lines),
      payments: [...this.payments.values()],
      series: Object.fromEntries(this.series),
      entries: [...this.entries.values()],
      reconciliations: [...this.reconciliations.values()],
      seriesEntries: Object.fromEntries(this.seriesEntries),
      billingProfile: this.billingProfile,
      attachments: this.persistableAttachments(),
      clock: this.clock,
    })
  }

  private persistableAttachments(): PersistedAttachment[] {
    const out: PersistedAttachment[] = []
    let used = 0
    for (const list of this.attachments.values()) {
      for (const { bytes, ...meta } of list) {
        // A file that does not fit stays session-only; smaller ones after it still fit.
        if (used + bytes.length > PERSIST_ATTACHMENT_BUDGET) continue
        used += bytes.length
        out.push({ ...meta, data: toBase64(bytes) })
      }
    }
    return out
  }

  // ── Receipts ──

  attachmentsOf(entryId: string): StoredAttachment[] {
    return this.attachments.get(entryId) ?? []
  }

  putAttachment(a: StoredAttachment): StoredAttachment {
    const list = this.attachments.get(a.entry_id) ?? []
    list.push(a)
    this.attachments.set(a.entry_id, list)
    return a
  }

  dropAttachment(entryId: string, id: string): void {
    const list = this.attachments.get(entryId)
    if (list) this.attachments.set(entryId, list.filter((a) => a.id !== id))
  }

  /** The API shape of an entry: stored fields plus the receipt count. */
  entryView(e: StoredEntry): Entry {
    return { ...e, attachment_count: this.attachmentsOf(e.id).length }
  }

  /** Receipt as a data: URL, for <img>/links in the browser demo (no server to ask). */
  attachmentDataUrl(entryId: string, id: string): string | null {
    const a = this.attachmentsOf(entryId).find((x) => x.id === id)
    return a ? `data:${a.mime_type};base64,${toBase64(a.bytes)}` : null
  }

  /** Replaces all state with a snapshot from toJSON(). */
  load(s: FinanceSnapshot): void {
    this.invoices.clear()
    this.lines.clear()
    this.payments.clear()
    this.series.clear()
    this.entries.clear()
    this.reconciliations.clear()
    this.seriesEntries.clear()
    this.attachments.clear()
    for (const { data, ...meta } of s.attachments ?? []) this.putAttachment({ ...meta, bytes: fromBase64(data) })
    for (const i of s.invoices ?? []) this.invoices.set(i.id, i)
    for (const [k, v] of Object.entries(s.lines ?? {})) this.lines.set(k, v)
    for (const p of s.payments ?? []) this.payments.set(p.id, p)
    for (const [k, v] of Object.entries(s.series ?? {})) this.series.set(k, v)
    for (const e of s.entries ?? []) this.entries.set(e.id, e)
    for (const r of s.reconciliations ?? []) this.reconciliations.set(r.id, r)
    for (const [k, v] of Object.entries(s.seriesEntries ?? {})) this.seriesEntries.set(k, v)
    this.billingProfile = structuredClone(s.billingProfile ?? DEFAULT_BILLING_PROFILE)
    this.clock = Math.max(this.clock, s.clock ?? 0)
  }

  // ── Phase 5: earnings + reconciliations ──────────────────────────────────

  /** Creates a manual entry the same way the Go service does. */
  addEntry(input: {
    id?: string
    kind: EntryKind
    amount_minor: number
    currency: string
    category: string
    entry_date: string
    description?: string
    notes?: string
    gig_id?: string | null
    source_kind?: EntrySource
    source_id?: string | null
    source_amount_minor?: number | null
    source_currency?: string | null
    source_description?: string
    auto_generated?: boolean
  }): StoredEntry {
    const now = this.stamp()
    const e: StoredEntry = {
      id: input.id ?? uuid(),
      kind: input.kind,
      amount_minor: input.amount_minor,
      currency: input.currency,
      category: input.category,
      entry_date: input.entry_date,
      description: input.description ?? '',
      notes: input.notes ?? '',
      gig_id: input.gig_id ?? null,
      status: 'active',
      auto_generated: input.auto_generated ?? false,
      source_kind: input.source_kind ?? 'manual',
      source_id: input.source_id ?? null,
      source_amount_minor: input.source_amount_minor ?? null,
      source_currency: input.source_currency ?? null,
      source_description: input.source_description ?? '',
      created_at: now,
      updated_at: now,
      deleted_at: null,
    }
    this.entries.set(e.id, e)
    return e
  }

  /**
   * Active gig-payment income lookup. Mirrors the Go partial unique index on
   * (gig_id) WHERE source_kind='gig_payment' AND status='active' AND
   * deleted_at IS NULL — at most one auto-generated income per gig.
   */
  findActiveGigIncome(gigId: string): StoredEntry | null {
    for (const e of this.entries.values()) {
      if (e.deleted_at || e.status !== 'active') continue
      if (e.source_kind !== 'gig_payment' || e.gig_id !== gigId) continue
      if (e.kind !== 'income' || !e.auto_generated) continue
      return e
    }
    return null
  }

  /** Calls findActiveGigIncome then creates one if missing. Idempotent. */
  upsertGigIncome(snap: {
    gigId: string
    amount_minor: number
    currency: string
    entry_date: string
    description: string
  }): { entry: StoredEntry; created: boolean } {
    const existing = this.findActiveGigIncome(snap.gigId)
    if (existing) {
      const same =
        existing.amount_minor === snap.amount_minor &&
        existing.currency === snap.currency &&
        existing.entry_date === snap.entry_date &&
        existing.description === snap.description
      if (same) return { entry: existing, created: false }
    }
    const entry = this.addEntry({
      kind: 'income',
      amount_minor: snap.amount_minor,
      currency: snap.currency,
      category: 'gig_fee',
      entry_date: snap.entry_date,
      description: snap.description,
      gig_id: snap.gigId,
      source_kind: 'gig_payment',
      source_id: snap.gigId,
      source_amount_minor: snap.amount_minor,
      source_currency: snap.currency,
      source_description: `gig:${snap.gigId}`,
      auto_generated: true,
    })
    return { entry, created: true }
  }

  /** Upserts a pending reconciliation row for one gig. At most one per gig. */
  upsertReconciliation(input: {
    gig_id: string
    entry_id: string
    reason: ReconciliationReason
    allowed_actions: ReconciliationAction[]
    gig_amount_minor: number
    gig_currency: string
    gig_payment_status: string
    entry_updated_at: string
  }): StoredReconciliation | null {
    for (const r of this.reconciliations.values()) {
      if (r.gig_id !== input.gig_id) continue
      if (r.status !== 'pending') continue
      // Refresh the existing row in place — there is at most one pending
      // per gig, mirroring the partial index in the migration.
      r.reason = input.reason
      r.allowed_actions = input.allowed_actions
      r.gig_amount_minor = input.gig_amount_minor
      r.gig_currency = input.gig_currency
      r.gig_payment_status = input.gig_payment_status
      r.entry_updated_at = input.entry_updated_at
      r.updated_at = this.stamp()
      return r
    }
    const now = this.stamp()
    const row: StoredReconciliation = {
      id: uuid(),
      gig_id: input.gig_id,
      entry_id: input.entry_id,
      reason: input.reason,
      allowed_actions: input.allowed_actions,
      gig_amount_minor: input.gig_amount_minor,
      gig_currency: input.gig_currency,
      gig_payment_status: input.gig_payment_status,
      entry_updated_at: input.entry_updated_at,
      status: 'pending',
      resolution: null,
      created_at: now,
      updated_at: now,
      resolved_at: null,
    }
    this.reconciliations.set(row.id, row)
    return row
  }

  /** Acknowledges every pending reconciliation for gigID as resolved: 'keep'. */
  acknowledgeReconciliations(gigId: string): void {
    for (const r of this.reconciliations.values()) {
      if (r.gig_id !== gigId || r.status !== 'pending') continue
      r.status = 'resolved'
      r.resolution = 'keep'
      r.updated_at = this.stamp()
      r.resolved_at = r.updated_at
    }
  }

  /**
   * Drives the gig-payment transition side of Phase 5. Replicates the Go
   * processor's behaviour without touching the gigs mock: callers (demo /
   * dev) detect fee or currency shifts when a paid gig is updated or its
   * payment reverts, and either insert a fresh auto-generated entry or
   * raise a reconciliation prompt.
   */
  processGigPaymentTransition(snap: {
    gig_id: string
    amount_minor: number
    currency: string
    entry_date: string
    description: string
    /** 'paid' on both sides of the update — no transition, no prompt. */
    prev_payment_status?: string
    next_payment_status?: string
  }): { metadata: { id: string; entry_id: string; reason: ReconciliationReason; allowed_actions: ReconciliationAction[]; updated_at: string } | null } {
    const prev = snap.prev_payment_status ?? 'unpaid'
    const next = snap.next_payment_status ?? 'paid'
    const becamePaid = prev !== 'paid' && next === 'paid'
    const leftPaid = prev === 'paid' && next !== 'paid'

    const entry = this.findActiveGigIncome(snap.gig_id)

    // Mirrors the Go processor: only create the first income row here —
    // once one exists, every later write (even one that also happens to
    // flip the gig to paid, e.g. a "keep"-resolved reversal paid again)
    // goes through the same reason-detection below instead of blindly
    // inserting a second active row for the same gig.
    if (becamePaid && !entry) {
      this.upsertGigIncome({
        gigId: snap.gig_id,
        amount_minor: snap.amount_minor,
        currency: snap.currency,
        entry_date: snap.entry_date,
        description: snap.description,
      })
      this.acknowledgeReconciliations(snap.gig_id)
      return { metadata: null }
    }

    if (!entry) return { metadata: null }

    let reason: ReconciliationReason | null = null
    if (leftPaid) reason = 'payment_reversed'
    else if (entry.source_currency != null && entry.source_currency !== snap.currency) reason = 'currency_changed'
    else if (entry.source_amount_minor != null && entry.source_amount_minor !== snap.amount_minor) reason = 'fee_changed'

    if (!reason) {
      // A payment_reversed decision may still be pending from an earlier
      // save (the gig left "paid" then, this write doesn't touch fee/
      // currency/status again) — leave it for the user instead of
      // silently acknowledging it while the gig is still unpaid.
      if (next === 'paid') this.acknowledgeReconciliations(snap.gig_id)
      return { metadata: null }
    }
    const allowed: ReconciliationAction[] =
      reason === 'payment_reversed'
        ? ['delete', 'void', 'keep']
        : ['update', 'keep']
    const row = this.upsertReconciliation({
      gig_id: snap.gig_id,
      entry_id: entry.id,
      reason,
      allowed_actions: allowed,
      gig_amount_minor: snap.amount_minor,
      gig_currency: snap.currency,
      gig_payment_status: next,
      entry_updated_at: entry.updated_at,
    })
    if (!row) return { metadata: null }
    return {
      metadata: {
        id: row.id,
        entry_id: row.entry_id,
        reason: row.reason,
        allowed_actions: row.allowed_actions,
        updated_at: row.updated_at,
      },
    }
  }
}
