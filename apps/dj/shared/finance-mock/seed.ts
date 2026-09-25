// Fictional seed data for the finance mocks: played gigs and invoices in
// every interesting state. Dates are offsets from `today` so the demo stays
// current; the Nitro dev mocks pin today to keep their data stable.
// Every name, address and ID here is a placeholder (example.* domains,
// zero-padded IDs). Never add real venues, promoters or tax numbers.

import type { Party } from '../../app/types/finance'
import type { FinanceMockDb, MockGig } from './db'
import { addDays, party } from './rules'

export const FINANCE_GIG_IDS = {
  domestic: '00000000-0000-4000-8000-000000000101',
  reverseCharge: '00000000-0000-4000-8000-000000000102',
  us: '00000000-0000-4000-8000-000000000103',
  incomplete: '00000000-0000-4000-8000-000000000104',
  credited: '00000000-0000-4000-8000-000000000105',
  unbilled: '00000000-0000-4000-8000-000000000106',
} as const

/** A billable played gig with the details the gig tracker also shows. */
export interface SeedGig {
  id: string
  daysAgo: number
  venue: string
  event_name: string
  city: string
  country: string
  fee_minor: number
  currency: string
  customer: Party
}

export const FINANCE_SEED_GIGS: SeedGig[] = [
  { id: FINANCE_GIG_IDS.domestic, daysAgo: 48, venue: 'Club Alpha', event_name: 'Resident Night', city: 'Berlin', country: 'DE', fee_minor: 80000, currency: 'EUR',
    customer: party({ legal_name: 'Alpha Events GmbH', company: 'Club Alpha', email: 'office@alpha.example', address_line1: '10 Sample Road', city: 'Berlin', postal_code: '10001', country: 'DE', vat_id: 'DE000000001' }) },
  { id: FINANCE_GIG_IDS.reverseCharge, daysAgo: 34, venue: 'Club Beta', event_name: 'Techno Night', city: 'Paris', country: 'FR', fee_minor: 150000, currency: 'EUR',
    customer: party({ legal_name: 'Beta Nights SAS', company: 'Club Beta', email: 'booking@beta.example', address_line1: '20 Rue Exemple', city: 'Paris', postal_code: '75000', country: 'FR', vat_id: 'FR00000000000' }) },
  { id: FINANCE_GIG_IDS.us, daysAgo: 13, venue: 'Gamma Hall', event_name: 'Sunday Party', city: 'New York', country: 'US', fee_minor: 250000, currency: 'USD',
    customer: party({ legal_name: 'Gamma Hall LLC', company: 'Gamma Hall', email: 'pay@gamma.example', address_line1: '30 Example Ave', city: 'New York', region: 'NY', postal_code: '10000', country: 'US', tax_id: '00-0000000' }) },
  { id: FINANCE_GIG_IDS.incomplete, daysAgo: 6, venue: 'Delta Room', event_name: 'Room One', city: 'London', country: 'GB', fee_minor: 120000, currency: 'EUR',
    customer: party({ legal_name: 'Delta Promotions', email: 'accounts@delta.example', country: 'GB' }) },
  { id: FINANCE_GIG_IDS.credited, daysAgo: 56, venue: 'Epsilon Festival', event_name: '', city: 'Amsterdam', country: 'NL', fee_minor: 180000, currency: 'EUR',
    customer: party({ legal_name: 'Epsilon Festival BV', company: 'Epsilon Festival', address_line1: '40 Voorbeeldstraat', city: 'Amsterdam', postal_code: '1000', country: 'NL', vat_id: 'NL000000000B01' }) },
  { id: FINANCE_GIG_IDS.unbilled, daysAgo: 5, venue: 'Zeta Garden', event_name: 'Open Air', city: 'Berlin', country: 'DE', fee_minor: 60000, currency: 'EUR',
    customer: party({ legal_name: 'Zeta Collective', address_line1: '50 Sample Lane', city: 'Berlin', postal_code: '10002', country: 'DE' }) },
]

/** "Venue — Event" label used on invoice lines. */
export function gigLabel(venue: string, eventName: string): string {
  return eventName ? `${venue} — ${eventName}` : venue
}

export function seedMockGigs(today: string): MockGig[] {
  return FINANCE_SEED_GIGS.map((g) => ({
    id: g.id,
    date: addDays(today, -g.daysAgo),
    label: gigLabel(g.venue, g.event_name),
    fee_minor: g.fee_minor,
    currency: g.currency,
    customer: structuredClone(g.customer),
  }))
}

const at = (date: string, time = '10:00:00') => `${date}T${time}Z`

/** Seeds invoices in every state; `gigDate` resolves a seeded gig's date. */
export function seedInvoices(db: FinanceMockDb, gigDate: (id: string) => string): void {
  const ids = FINANCE_GIG_IDS

  // Paid domestic invoice (DE → DE, 19%).
  const d1 = gigDate(ids.domestic)
  const paid = db.newDraft(ids.domestic)
  db.issue(paid, at(addDays(d1, 1)))
  db.addPayment(paid.id, { currency: 'EUR', amount_minor: paid.net_payable_minor, kind: 'payment', status: 'completed', method: 'bank transfer', received_at: at(addDays(d1, 12), '00:00:00') })
  paid.status = 'paid'
  paid.paid_at = at(addDays(d1, 12), '00:00:00')

  // Overdue EU reverse charge (DE → FR) with a received deposit.
  const d2 = gigDate(ids.reverseCharge)
  const rc = db.newDraft(ids.reverseCharge, { due_at: at(addDays(d2, 14), '00:00:00') })
  db.issue(rc, at(addDays(d2, 1)))
  db.addPayment(rc.id, { currency: 'EUR', amount_minor: 50000, kind: 'deposit', status: 'completed', method: 'bank transfer', received_at: at(addDays(d2, -21), '00:00:00') })

  // US customer, no tax, 30% withholding, a pending payment.
  const d3 = gigDate(ids.us)
  const us = db.newDraft(ids.us, {
    vat_treatment: 'none', tax_rate_bps: 0, tax_note: '', withholding_rate_bps: 3000, due_at: at(addDays(d3, 30), '00:00:00'),
  })
  db.recompute(us)
  db.issue(us, at(addDays(d3, 1)))
  db.addPayment(us.id, { currency: 'USD', amount_minor: 100000, kind: 'payment', status: 'pending', method: 'wire' })

  // Draft with an incomplete customer (issue-check problems).
  db.newDraft(ids.incomplete)

  // Credited invoice and its credit note.
  const credited = db.newDraft(ids.credited)
  db.issue(credited, at(addDays(gigDate(ids.credited), 1)))
  db.creditNoteFor(credited, 'Festival cancelled the slot')
  credited.status = 'credited'
  credited.updated_at = db.stamp()
}
