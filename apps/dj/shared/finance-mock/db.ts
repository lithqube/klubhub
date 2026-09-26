// In-memory invoice store implementing the invoicing contract
// (docs/INVOICING.md). One instance backs the Nitro dev mocks, another the
// browser demo; the demo persists it with toJSON()/load().

import type { Invoice, InvoiceLine, IssueProblem, Party, Payment, TaxSuggestion } from '../../app/types/finance'
import { applyBps, addDays, DEFAULT_SUPPLIER, EU, party, suggest, uuid, type Supplier } from './rules'

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
  clock: number
}

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
  readonly supplier: Supplier
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
    return { ...structuredClone(inv), ...b }
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
      quantity: 1, unit_minor: gig.fee_minor, tax_bps: inv.tax_rate_bps, line_total_minor: gig.fee_minor, created_at: now,
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
      clock: this.clock,
    })
  }

  /** Replaces all state with a snapshot from toJSON(). */
  load(s: FinanceSnapshot): void {
    this.invoices.clear()
    this.lines.clear()
    this.payments.clear()
    this.series.clear()
    for (const i of s.invoices ?? []) this.invoices.set(i.id, i)
    for (const [k, v] of Object.entries(s.lines ?? {})) this.lines.set(k, v)
    for (const p of s.payments ?? []) this.payments.set(p.id, p)
    for (const [k, v] of Object.entries(s.series ?? {})) this.series.set(k, v)
    this.clock = Math.max(this.clock, s.clock ?? 0)
  }
}
