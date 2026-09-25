// Gig choices for the create-invoice picker: billable gigs first, and gigs
// that cannot get a new invoice stay visible but disabled with the reason.

import type { Invoice, InvoiceGigOption } from '../types/finance'
import { formatMinor, parseMoney } from './money'
import { dateOnly, invoiceNumberLabel, shortDate } from './invoiceDisplay'

export const BILLABLE_GIG_STATUSES = ['confirmed', 'advanced', 'played'] as const

export interface GigChoice {
  id: string
  label: string
  date: string
  dateLabel: string
  status: string
  currency: string
  feeMinor: number
  feeLabel: string
  group: 'billable' | 'other'
  /** '' when the gig can be invoiced. */
  disabledReason: string
}

export function gigFeeMinor(g: Pick<InvoiceGigOption, 'fee_amount' | 'fee_currency'>): number {
  return parseMoney(String(g.fee_amount ?? 0), g.fee_currency || 'EUR') ?? 0
}

export function buildGigChoices(
  gigs: InvoiceGigOption[],
  activeByGig: Record<string, Pick<Invoice, 'invoice_number' | 'kind'>>,
): GigChoice[] {
  const choices = gigs.map<GigChoice>((g) => {
    const currency = g.fee_currency || 'EUR'
    const feeMinor = gigFeeMinor(g)
    const active = activeByGig[g.id]
    let disabledReason = ''
    if (active) disabledReason = `Already invoiced (${invoiceNumberLabel(active)})`
    else if (feeMinor <= 0) disabledReason = 'No fee set on the gig'
    return {
      id: g.id,
      label: g.label,
      date: dateOnly(g.date),
      dateLabel: shortDate(g.date),
      status: g.status,
      currency,
      feeMinor,
      feeLabel: formatMinor(feeMinor, currency),
      group: (BILLABLE_GIG_STATUSES as readonly string[]).includes(g.status) ? 'billable' : 'other',
      disabledReason,
    }
  })
  // Billable first; within a group, invoiceable before disabled, then newest first.
  return choices.sort((a, b) => {
    if (a.group !== b.group) return a.group === 'billable' ? -1 : 1
    if (!a.disabledReason !== !b.disabledReason) return a.disabledReason ? 1 : -1
    return b.date.localeCompare(a.date)
  })
}

export function matchesGigQuery(c: GigChoice, query: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  return `${c.label} ${c.dateLabel} ${c.date} ${c.status}`.toLowerCase().includes(q)
}
