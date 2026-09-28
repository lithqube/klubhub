// Finance / invoicing types. Mirrors docs/INVOICING.md §2 exactly; change
// both sides together. Money is always integer minor units (`*_minor`).

export type VatTreatment =
  | 'domestic'
  | 'reverse_charge'
  | 'exempt'
  | 'outside_scope'
  | 'us_sales_tax'
  | 'none'

export type InvoiceKind = 'invoice' | 'credit_note'

export type InvoiceStatus = 'draft' | 'issued' | 'paid' | 'cancelled' | 'credited' | 'corrected'

export interface Party {
  contact_id: string | null
  legal_name: string
  company: string
  email: string
  address_line1: string
  address_line2: string
  city: string
  region: string
  postal_code: string
  country: string
  vat_id: string
  tax_id: string
  is_business: boolean
}

export interface TaxBreakdownRow {
  rate_bps: number
  taxable_minor: number
  tax_minor: number
}

export interface Invoice {
  id: string
  kind: InvoiceKind
  gig_id: string
  credits_invoice_id: string | null
  replaced_by_invoice_id: string | null
  invoice_number: string | null
  number_prefix: string
  number_seq: number | null
  currency: string
  status: InvoiceStatus
  supply_date: string | null
  issued_at: string | null
  due_at: string | null
  paid_at: string | null
  payment_ref: string
  internal_notes: string
  customer: Party
  billing_profile: Record<string, unknown> | null
  vat_treatment: VatTreatment
  tax_rate_bps: number
  tax_note: string
  subtotal_minor: number
  tax_minor: number
  total_minor: number
  withholding_rate_bps: number
  withholding_minor: number
  net_payable_minor: number
  tax_breakdown: TaxBreakdownRow[]
  received_minor: number
  pending_minor: number
  outstanding_minor: number
  updated_at: string
  created_at: string
}

export interface InvoiceLine {
  id: string
  invoice_id: string
  sort_order: number
  description: string
  quantity: number
  unit_minor: number
  tax_bps: number
  line_total_minor: number
  created_at: string
}

export interface IssueProblem {
  /** Dotted path of the offending field, e.g. "customer.vat_id". */
  field: string
  message: string
}

export interface IssueCheck {
  ready: boolean
  problems: IssueProblem[]
}

export interface TaxSuggestion {
  vat_treatment: VatTreatment
  tax_rate_bps: number
  tax_note: string
  reason: string
}

export interface InvoiceSummary {
  currency: string
  draft_count: number
  issued_count: number
  paid_count: number
  outstanding_minor: number
  paid_minor: number
}

export type PaymentKind = 'deposit' | 'payment' | 'refund'
export type PaymentRecordStatus = 'pending' | 'completed' | 'failed' | 'refunded'

export interface Payment {
  id: string
  invoice_id: string
  currency: string
  amount_minor: number
  kind: PaymentKind
  status: PaymentRecordStatus
  method: string
  reference: string
  received_at: string | null
  created_at: string
  updated_at: string
}

// ── Request bodies ──────────────────────────────────────────────────────

export interface InvoiceCreateInput {
  gig_id: string
  customer?: Party
  vat_treatment?: VatTreatment
  tax_rate_bps?: number
  withholding_rate_bps?: number
  supply_date?: string
  due_at?: string
  number_prefix?: string
}

export interface InvoiceUpdateInput {
  customer: Party
  vat_treatment: VatTreatment
  tax_rate_bps: number
  tax_note: string
  withholding_rate_bps: number
  /** YYYY-MM-DD (the API rejects timestamps here). */
  supply_date: string | null
  /** RFC 3339 or YYYY-MM-DD. */
  due_at: string | null
  number_prefix: string
  internal_notes: string
}

export interface PaymentCreateInput {
  kind: PaymentKind
  amount_minor: number
  method?: string
  reference?: string
  received_at?: string | null
  /** UI-only: the money has already arrived → mark the payment completed. */
  received?: boolean
}

export interface InvoiceListQuery {
  currency?: string
  status?: InvoiceStatus
  gig_id?: string
  kind?: InvoiceKind
}

// ── UI-level derived types ──────────────────────────────────────────────

/** Status as shown to the user: `overdue` and `void` are derived. */
export type InvoiceDisplayStatus = 'draft' | 'issued' | 'overdue' | 'paid' | 'void'

export type InvoiceListFilter = 'all' | InvoiceDisplayStatus

/** Error codes from the finance API error body `{error, message, problems?}`. */
export type FinanceErrorCode =
  | 'validation_failed'
  | 'bad_request'
  | 'not_found'
  | 'conflict'
  | 'bad_state'
  | 'not_issuable'
  | 'invoice_not_payable'
  | 'exceeds_balance'
  | 'unavailable'
  | 'network'
  | 'unknown'

export interface FinanceErrorBody {
  error: string
  message: string
  problems?: IssueProblem[]
}

/** Irreversible actions confirmed by InvoiceConfirmDialog. */
export type InvoiceConfirmAction = 'issue' | 'cancel' | 'credit' | 'correct' | 'pay'

/** The tax slice of a draft edited by InvoiceTaxFields. */
export interface InvoiceTaxFieldsValue {
  vat_treatment: VatTreatment
  tax_rate_bps: number
  tax_note: string
  withholding_rate_bps: number
}

/** Minimal gig projection the invoice UI needs (gig picker, list fallback). */
export interface InvoiceGigOption {
  id: string
  date: string
  label: string
  status: string
  fee_amount: number
  fee_currency: string
}

// ── Phase 5 earnings (FIN-01…FIN-10) ──────────────────────────────────────
//
// Money is always integer minor units (`*_minor`). Currencies are 3-letter
// ISO 4217 codes (uppercased). Entry dates are YYYY-MM-DD; reconciliation
// timestamps are RFC 3339 nanosecond strings.
//
// FIN-10 keeps tax / VAT out of scope: the UI does not compute or display
// any tax/VAT amounts, and this comment block is the authoritative note.

export type EntryKind = 'income' | 'expense'
export type EntryStatus = 'active' | 'voided'
export type EntrySource = 'manual' | 'gig_payment' | 'invoice_payment'
export type SummaryScope = 'month' | 'year'
export type ProfitLossScope = 'gig' | 'month' | 'year'

/** A single ledger entry (income or expense). Multi-currency: never converted. */
export interface Entry {
  id: string
  kind: EntryKind
  amount_minor: number
  currency: string
  category: string
  entry_date: string
  description: string
  notes: string
  gig_id: string | null
  status: EntryStatus
  auto_generated: boolean
  source_kind: EntrySource
  source_id: string | null
  source_amount_minor: number | null
  source_currency: string | null
  source_description: string
  created_at: string
  updated_at: string
  deleted_at: string | null
}

/** Server-side filter accepted by GET /api/v1/finance/entries. Empty fields drop. */
export interface EntryFilter {
  kind?: EntryKind
  status?: EntryStatus
  currency?: string
  category?: string
  gig_id?: string
  from?: string
  to?: string
}

/** Currency-grouped totals — multi-currency without conversion (FIN-09). */
export interface EntryTotals {
  currency: string
  income_minor: number
  expense_minor: number
}

/** Profit/loss per currency (FIN-07). */
export interface ProfitLossTotals {
  currency: string
  income_minor: number
  expense_minor: number
  profit_loss_minor: number
}

/** Mutable body for POST /api/v1/finance/entries. */
export interface EntryCreateInput {
  kind: EntryKind
  amount_minor: number
  currency: string
  category: string
  entry_date: string
  description?: string
  notes?: string
  gig_id?: string | null
}

/** Mutable body for PUT /api/v1/finance/entries/{id} — requires updated_at token. */
export interface EntryUpdateInput {
  kind: EntryKind
  amount_minor: number
  currency: string
  category: string
  entry_date: string
  description?: string
  notes?: string
  gig_id?: string | null
  updated_at: string
}

/** DELETE /api/v1/finance/entries/{id} body — requires updated_at token. */
export interface EntryDeleteInput {
  updated_at: string
}

/** POST /api/v1/finance/entries/{id}/void body — requires updated_at token. */
export interface EntryVoidInput {
  updated_at: string
}

// ── Reconciliations (FIN-04 / FIN-05) ────────────────────────────────────

export type ReconciliationReason = 'fee_changed' | 'currency_changed' | 'payment_reversed'
export type ReconciliationAction = 'update' | 'delete' | 'void' | 'keep'
export type ReconciliationStatus = 'pending' | 'resolved'

/** Durable decision surfaced when a gig update or payment revert rewrites the source snapshot. */
export interface EntryReconciliation {
  id: string
  gig_id: string
  entry_id: string
  reason: ReconciliationReason
  allowed_actions: ReconciliationAction[]
  gig_amount_minor: number
  gig_currency: string
  gig_payment_status: string
  entry_updated_at: string
  status: ReconciliationStatus
  resolution: ReconciliationAction | null
  created_at: string
  updated_at: string
  resolved_at: string | null
}

/** Body for POST /api/v1/finance/reconciliations/{id}/resolve. */
export interface ResolveReconciliationInput {
  action: ReconciliationAction
  updated_at: string
}

/**
 * Compact reconciliation metadata returned inline with a gig update response
 * when the transition processor raises a prompt in the same write. The UI
 * mirrors the durable GET against this for the freshly-created case; the
 * GET remains the source of truth after a page refresh.
 */
export interface GigFinanceReconciliation {
  id: string
  entry_id: string
  reason: ReconciliationReason
  allowed_actions: ReconciliationAction[]
  updated_at: string
}
