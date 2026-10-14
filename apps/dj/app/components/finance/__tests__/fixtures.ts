// Fictional invoice fixtures for component tests (Club Alpha, example.com).
import type { Entry, EntryAttachment, Invoice } from '../../../types/finance'
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
    buyer_reference: '',
    purchase_order_ref: '',
    contract_ref: '',
    payment_terms: '',
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

// ── Receipts on ledger entries ──
export function makeEntry(over: Partial<Entry> = {}): Entry {
  return {
    id: 'e-1', kind: 'expense', amount_minor: 5000, currency: 'EUR', category: 'gear',
    entry_date: '2026-09-15', description: '', notes: 'New cable', gig_id: null,
    status: 'active', auto_generated: false, source_kind: 'manual', source_id: null,
    source_amount_minor: null, source_currency: null, source_description: '',
    created_at: '2026-09-15T10:00:00Z', updated_at: '2026-09-15T10:00:00Z', deleted_at: null,
    attachment_count: 0,
    ...over,
  }
}

export function makeAttachment(over: Partial<EntryAttachment> = {}): EntryAttachment {
  return {
    id: 'a-1', entry_id: 'e-1', filename: 'receipt.png', mime_type: 'image/png', size_bytes: 2048,
    checksum_sha256: 'abc', created_at: '2026-09-15T10:05:00Z',
    ...over,
  }
}

/** A File of `size` bytes (the content is irrelevant to the client checks). */
export function makeFile(name: string, type: string, size = 1024): File {
  const file = new File([new Uint8Array(Math.min(size, 16))], name, { type })
  if (size > 16) Object.defineProperty(file, 'size', { value: size })
  return file
}

/** The error shape $fetch rejects with for an API error body. */
export function httpError(status: number, error: string, message: string): Error {
  return Object.assign(new Error(`HTTP ${status}`), { statusCode: status, data: { error, message } })
}

export function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
