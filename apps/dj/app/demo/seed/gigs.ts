// Fictional gigs, venues and contacts for the demo. The six played gigs
// share ids and customers with the finance seed (shared/finance-mock).

import { FINANCE_GIG_IDS, FINANCE_SEED_GIGS } from '../../../shared/finance-mock/seed'
import { party } from '../../../shared/finance-mock/rules'
import type { Party } from '../../types/finance'
import type { Contact, Gig, GigStatus, PaymentStatus, Venue } from '../../types/gig'
import type { GigLinks } from '../types'
import { daysFrom } from '../lib/util'
import { DEMO_TRACKLIST_IDS } from './tracklists'

const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`

interface GigSpec {
  id: string
  days: number
  venue: string
  event_name: string
  city: string
  country: string
  fee: number
  currency: string
  status: GigStatus
  payment_status: PaymentStatus
  promoter: string
  email: string
  notes?: string
}

const PLAYED_PAYMENT: Record<string, PaymentStatus> = {
  [FINANCE_GIG_IDS.domestic]: 'paid',
  [FINANCE_GIG_IDS.reverseCharge]: 'deposit_paid',
  [FINANCE_GIG_IDS.us]: 'unpaid',
  [FINANCE_GIG_IDS.incomplete]: 'unpaid',
  [FINANCE_GIG_IDS.credited]: 'waived',
  [FINANCE_GIG_IDS.unbilled]: 'unpaid',
}

const UPCOMING: (GigSpec & { customer: Party })[] = [
  { id: id(201), days: 9, venue: 'Club Alpha', event_name: 'Resident Night', city: 'Berlin', country: 'DE', fee: 800, currency: 'EUR',
    status: 'confirmed', payment_status: 'unpaid', promoter: 'Alpha Events GmbH', email: 'office@alpha.example',
    customer: FINANCE_SEED_GIGS[0]!.customer },
  { id: id(202), days: 23, venue: 'Beta Warehouse', event_name: 'Warehouse Session', city: 'Hamburg', country: 'DE', fee: 1200, currency: 'EUR',
    status: 'advanced', payment_status: 'deposit_paid', promoter: 'Beta Warehouse Collective', email: 'bookings@beta-warehouse.example',
    notes: 'Load-in 22:00. Two media players and a four-channel mixer confirmed.',
    customer: party({ legal_name: 'Beta Warehouse Collective', address_line1: '2 Example Quay', city: 'Hamburg', postal_code: '20000', country: 'DE', email: 'bookings@beta-warehouse.example' }) },
  { id: id(203), days: 41, venue: 'Gamma Hall', event_name: 'Autumn Showcase', city: 'New York', country: 'US', fee: 2000, currency: 'USD',
    status: 'inquiry', payment_status: 'unpaid', promoter: 'Gamma Hall LLC', email: 'pay@gamma.example',
    customer: FINANCE_SEED_GIGS[2]!.customer },
  { id: id(204), days: -20, venue: 'Theta Bar', event_name: 'Listening Session', city: 'Vienna', country: 'AT', fee: 300, currency: 'EUR',
    status: 'cancelled', payment_status: 'waived', promoter: 'Theta Bar OG', email: 'hello@theta.example',
    customer: party({ legal_name: 'Theta Bar OG', country: 'AT' }) },
]

function gigFrom(s: GigSpec, now: Date): Gig {
  const created = daysFrom(now, Math.min(s.days, 0) - 30, 12)
  return {
    id: s.id,
    date: daysFrom(now, s.days),
    venue: s.venue,
    city: s.city,
    country: s.country,
    event_name: s.event_name,
    promoter_name: s.promoter,
    promoter_email: s.email,
    promoter_phone: '+00 000 000 0000',
    fee_amount: s.fee,
    fee_currency: s.currency,
    set_length_minutes: 120,
    notes: s.notes ?? '',
    status: s.status,
    payment_status: s.payment_status,
    gig_reader_venue_id: null,
    gig_reader_contact_id: null,
    created_at: created,
    updated_at: created,
    deleted_at: null,
  }
}

export const VENUE_IDS = {
  alpha: id(401), beta: id(402), gamma: id(403), delta: id(404), epsilon: id(405), zeta: id(406), betaWarehouse: id(407), theta: id(408),
}
export const CONTACT_IDS = { alex: id(501), bea: id(502), casey: id(503), dana: id(504) }

function venue(vid: string, name: string, city: string, country: string, capacity: number, stamp: string): Venue {
  return {
    id: vid, name, city, country, capacity, website: `https://${name.toLowerCase().replace(/\s+/g, '-')}.example.com`,
    tech_contact_name: 'Tech Example', tech_contact_email: 'tech@example.com', tech_contact_phone: '+00 000 000 0000',
    notes: '', created_at: stamp, updated_at: stamp, deleted_at: null,
  }
}

function contact(cid: string, name: string, company: string, email: string, type: Contact['type'], stamp: string): Contact {
  return { id: cid, name, company, email, phone: '+00 000 000 0000', type, notes: '', created_at: stamp, updated_at: stamp, deleted_at: null }
}

export function seedGigs(now: Date) {
  const stamp = daysFrom(now, -90, 12)
  const played: GigSpec[] = FINANCE_SEED_GIGS.map((g) => ({
    id: g.id, days: -g.daysAgo, venue: g.venue, event_name: g.event_name, city: g.city, country: g.country,
    fee: g.fee_minor / 100, currency: g.currency, status: 'played', payment_status: PLAYED_PAYMENT[g.id] ?? 'unpaid',
    promoter: g.customer.legal_name, email: g.customer.email,
  }))
  const gigs = [...played, ...UPCOMING].map((s) => gigFrom(s, now)).sort((a, b) => b.date.localeCompare(a.date))

  const customers: Record<string, Party> = {}
  for (const g of FINANCE_SEED_GIGS) customers[g.id] = structuredClone(g.customer)
  for (const g of UPCOMING) customers[g.id] = structuredClone(g.customer)

  const venues = [
    venue(VENUE_IDS.alpha, 'Club Alpha', 'Berlin', 'DE', 450, stamp),
    venue(VENUE_IDS.beta, 'Club Beta', 'Paris', 'FR', 600, stamp),
    venue(VENUE_IDS.gamma, 'Gamma Hall', 'New York', 'US', 1200, stamp),
    venue(VENUE_IDS.delta, 'Delta Room', 'London', 'GB', 300, stamp),
    venue(VENUE_IDS.epsilon, 'Epsilon Festival', 'Amsterdam', 'NL', 5000, stamp),
    venue(VENUE_IDS.zeta, 'Zeta Garden', 'Berlin', 'DE', 800, stamp),
    venue(VENUE_IDS.betaWarehouse, 'Beta Warehouse', 'Hamburg', 'DE', 900, stamp),
    venue(VENUE_IDS.theta, 'Theta Bar', 'Vienna', 'AT', 120, stamp),
  ]
  const contacts = [
    contact(CONTACT_IDS.alex, 'Alex Example', 'Alpha Events GmbH', 'alex@alpha.example', 'promoter', stamp),
    contact(CONTACT_IDS.bea, 'Bea Sample', 'Beta Nights SAS', 'bea@beta.example', 'promoter', stamp),
    contact(CONTACT_IDS.casey, 'Casey Placeholder', 'Example Agency', 'casey@agency.example', 'agent', stamp),
    contact(CONTACT_IDS.dana, 'Dana Demo', 'Gamma Hall LLC', 'dana@gamma.example', 'promoter', stamp),
  ]

  const gigLinks: Record<string, GigLinks> = {
    [FINANCE_GIG_IDS.domestic]: {
      venues: [{ venue_id: VENUE_IDS.alpha, is_primary: true }],
      contacts: [{ contact_id: CONTACT_IDS.alex, role: 'promoter' }],
      tracklists: [DEMO_TRACKLIST_IDS.resident],
    },
    [FINANCE_GIG_IDS.reverseCharge]: {
      venues: [{ venue_id: VENUE_IDS.beta, is_primary: true }],
      contacts: [{ contact_id: CONTACT_IDS.bea, role: 'promoter' }, { contact_id: CONTACT_IDS.casey, role: 'agent' }],
      tracklists: [DEMO_TRACKLIST_IDS.warehouse],
    },
    [FINANCE_GIG_IDS.unbilled]: {
      venues: [{ venue_id: VENUE_IDS.zeta, is_primary: true }],
      contacts: [],
      tracklists: [DEMO_TRACKLIST_IDS.openAir],
    },
  }

  return { gigs, customers, venues, contacts, gigLinks }
}
