import type { InvoiceSummary } from '../../../../../app/types/finance'
import { invoices, serialize } from '../-mockDb'

// §4.1: invoices only (credit notes excluded). paid_minor = net payable of
// paid invoices + received (clamped 0…net payable) of issued invoices.
export default defineEventHandler(() => {
  const out: Record<string, InvoiceSummary> = {}
  for (const stored of invoices.values()) {
    if (stored.kind !== 'invoice') continue
    const inv = serialize(stored)
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
  return { data: out }
})
