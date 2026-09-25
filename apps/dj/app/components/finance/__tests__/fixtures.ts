// Fictional invoice fixtures for component tests (Club Alpha, example.com).
import type { Invoice } from '../../../types/finance'
import { emptyParty } from '../../../utils/invoiceDisplay'

export function makeInvoice(overrides: Partial<Invoice> = {}): Invoice {
  return {
    id: 'inv-1',
    kind: 'invoice',
    gig_id: '00000000-0000-4000-8000-000000000001',
    credits_invoice_id: null,
    replaced_by_invoice_id: null,
    invoice_number: 'INV-0001-EUR',
    number_prefix: 'INV',
    number_seq: 1,
    currency: 'EUR',
    status: 'issued',
    supply_date: '2026-09-20',
    issued_at: '2026-09-21T10:00:00Z',
    due_at: '2999-10-04T00:00:00Z',
    paid_at: null,
    payment_ref: '',
    internal_notes: '',
    customer: { ...emptyParty(), legal_name: 'Alpha Events', company: 'Club Alpha', email: 'office@example.com' },
    billing_profile: null,
    vat_treatment: 'domestic',
    tax_rate_bps: 1900,
    tax_note: '',
    subtotal_minor: 100000,
    tax_minor: 19000,
    total_minor: 119000,
    withholding_rate_bps: 0,
    withholding_minor: 0,
    net_payable_minor: 119000,
    tax_breakdown: [{ rate_bps: 1900, taxable_minor: 100000, tax_minor: 19000 }],
    received_minor: 0,
    pending_minor: 0,
    outstanding_minor: 119000,
    updated_at: '2026-09-25T10:00:00Z',
    created_at: '2026-09-25T10:00:00Z',
    ...overrides,
  }
}
