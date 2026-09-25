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
