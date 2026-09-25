// Pure presentation helpers for invoices: derived status, badge mapping,
// labels. Kept out of components so the list, the sheet and the gig strip
// agree on what "overdue" or "void" means.

import type {
  Invoice,
  InvoiceDisplayStatus,
  InvoiceListFilter,
  InvoiceStatus,
  Party,
} from '../types/finance'

/** Today as YYYY-MM-DD in the viewer's local time zone. */
export function todayIso(now: Date = new Date()): string {
  const y = now.getFullYear()
  const m = String(now.getMonth() + 1).padStart(2, '0')
  const d = String(now.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

/** First 10 chars of an ISO date/time ("2026-09-25T00:00:00Z" → "2026-09-25"). */
export function dateOnly(value: string | null | undefined): string {
  return value ? value.slice(0, 10) : ''
}

/** Adds days to a YYYY-MM-DD date in UTC, returning YYYY-MM-DD. */
export function addDays(isoDate: string, days: number): string {
  const d = new Date(`${dateOnly(isoDate)}T00:00:00Z`)
  if (Number.isNaN(d.getTime())) return ''
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}

/** YYYY-MM-DD → RFC 3339 midnight UTC (the Go API decodes time.Time). */
export function toApiTime(isoDate: string | null | undefined): string | null {
  const d = dateOnly(isoDate)
  return d ? `${d}T00:00:00Z` : null
}

const VOID_STATUSES: InvoiceStatus[] = ['cancelled', 'credited', 'corrected']

export function invoiceDisplayStatus(inv: Pick<Invoice, 'status' | 'due_at'>, today = todayIso()): InvoiceDisplayStatus {
  if (VOID_STATUSES.includes(inv.status)) return 'void'
  if (inv.status === 'issued') {
    const due = dateOnly(inv.due_at)
    return due && due < today ? 'overdue' : 'issued'
  }
  return inv.status === 'paid' ? 'paid' : 'draft'
}

export const DISPLAY_STATUS_BADGE: Record<InvoiceDisplayStatus, string> = {
  draft: 'badge-draft',
  issued: 'badge-scheduled',
  overdue: 'badge-failed',
  paid: 'badge-ready',
  void: 'badge-archived',
}

export const DISPLAY_STATUS_ACCENT: Record<InvoiceDisplayStatus, string> = {
  draft: 'accent-bar-draft',
  issued: 'accent-bar-scheduled',
  overdue: 'accent-bar-failed',
  paid: 'accent-bar-ready',
  void: 'accent-bar-archived',
}

/** The raw status label, always text so status is never color-only. */
export function statusLabel(inv: Pick<Invoice, 'status' | 'due_at' | 'kind'>, today = todayIso()): string {
  const display = invoiceDisplayStatus(inv, today)
  if (display === 'void') return inv.status.toUpperCase()
  return display.toUpperCase()
}

export const LIST_FILTERS: { value: InvoiceListFilter; label: string }[] = [
  { value: 'all', label: 'ALL' },
  { value: 'draft', label: 'DRAFT' },
  { value: 'issued', label: 'ISSUED' },
  { value: 'overdue', label: 'OVERDUE' },
  { value: 'paid', label: 'PAID' },
  { value: 'void', label: 'VOID' },
]

export function matchesFilter(inv: Invoice, filter: InvoiceListFilter, today = todayIso()): boolean {
  if (filter === 'all') return true
  return invoiceDisplayStatus(inv, today) === filter
}

/** An invoice that still "owns" its gig: a new one would be a duplicate. */
export function isActiveInvoice(inv: Pick<Invoice, 'kind' | 'status'>): boolean {
  return inv.kind === 'invoice' && (inv.status === 'draft' || inv.status === 'issued' || inv.status === 'paid')
}

export function partyDisplayName(p: Party | null | undefined): string {
  if (!p) return ''
  return (p.company || p.legal_name || '').trim()
}

export function invoiceNumberLabel(inv: Pick<Invoice, 'invoice_number' | 'kind'>): string {
  if (inv.invoice_number) return inv.invoice_number
  return inv.kind === 'credit_note' ? 'CREDIT NOTE DRAFT' : 'DRAFT'
}

/** Signed amount for lists: credit notes reverse money, so they read negative. */
export function signedTotal(inv: Pick<Invoice, 'kind' | 'total_minor'>): number {
  return inv.kind === 'credit_note' ? -Math.abs(inv.total_minor) : inv.total_minor
}

/** Short date for rows ("25 SEP 2026"). */
export function shortDate(value: string | null | undefined): string {
  const d = dateOnly(value)
  if (!d) return '—'
  const date = new Date(`${d}T00:00:00Z`)
  if (Number.isNaN(date.getTime())) return d
  return date
    .toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric', timeZone: 'UTC' })
    .toUpperCase()
}

export function emptyParty(): Party {
  return {
    contact_id: null,
    legal_name: '',
    company: '',
    email: '',
    address_line1: '',
    address_line2: '',
    city: '',
    region: '',
    postal_code: '',
    country: '',
    vat_id: '',
    tax_id: '',
    is_business: true,
  }
}
