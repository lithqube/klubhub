// /api/v1/gigs, /venues, /contacts — gig tracker with links, calendar feed
// and a generated booking-confirmation PDF.

import { uuid } from '../../../shared/finance-mock/rules'
import type { Contact, Gig, GigStatus, PaymentStatus, Venue } from '../../types/gig'
import { pdfBlob } from '../lib/pdf'
import { gigsIcs } from '../lib/util'
import type { DemoRouter } from '../router'
import { apiError, json, noContent, notFound } from '../router'
import type { DemoContext, DemoRequest, GigLinks } from '../types'

const STATUSES: GigStatus[] = ['inquiry', 'confirmed', 'advanced', 'played', 'cancelled']
const PAYMENT: PaymentStatus[] = ['unpaid', 'deposit_paid', 'paid', 'overdue', 'waived']
const FIELDS = [
  'venue', 'city', 'country', 'event_name', 'promoter_name', 'promoter_email', 'promoter_phone',
  'fee_currency', 'notes',
] as const

const body = (req: DemoRequest) => (req.body && typeof req.body === 'object' ? req.body : {}) as Record<string, unknown>
const has = (q: URLSearchParams, k: string) => (q.get(k) ?? '') !== ''

function links(c: DemoContext, id: string): GigLinks {
  return (c.state.gigLinks[id] ??= { venues: [], contacts: [], tracklists: [] })
}

/** Validates and applies writable fields; returns an error message or ''. */
function apply(g: Gig, b: Record<string, unknown>): string {
  if (b.date !== undefined) {
    const d = String(b.date)
    if (Number.isNaN(Date.parse(d))) return 'date: must be RFC 3339'
    g.date = d.length === 10 ? `${d}T00:00:00Z` : d
  }
  for (const k of FIELDS) if (b[k] !== undefined && b[k] !== null) g[k] = String(b[k])
  if (b.fee_amount !== undefined) {
    const fee = Number(b.fee_amount)
    if (!Number.isFinite(fee) || fee < 0) return 'fee_amount: must be a positive number'
    g.fee_amount = fee
  }
  if (b.set_length_minutes !== undefined) g.set_length_minutes = Number(b.set_length_minutes) || 0
  if (b.status !== undefined) {
    if (!STATUSES.includes(b.status as GigStatus)) return 'status: unknown status'
    g.status = b.status as GigStatus
  }
  if (b.payment_status !== undefined) {
    if (!PAYMENT.includes(b.payment_status as PaymentStatus)) return 'payment_status: unknown status'
    g.payment_status = b.payment_status as PaymentStatus
  }
  g.fee_currency = (g.fee_currency || 'EUR').toUpperCase()
  return ''
}

function filterGigs(gigs: Gig[], q: URLSearchParams): Gig[] {
  const like = (v: string, k: string) => v.toLowerCase().includes((q.get(k) ?? '').toLowerCase())
  return gigs
    .filter((g) => !has(q, 'status') || g.status === q.get('status'))
    .filter((g) => !has(q, 'venue') || like(g.venue, 'venue'))
    .filter((g) => !has(q, 'city') || like(g.city, 'city'))
    .filter((g) => !has(q, 'fee_min') || g.fee_amount >= Number(q.get('fee_min')))
    .filter((g) => !has(q, 'fee_max') || g.fee_amount <= Number(q.get('fee_max')))
    .filter((g) => !has(q, 'from') || g.date.slice(0, 10) >= q.get('from')!.slice(0, 10))
    .filter((g) => !has(q, 'to') || g.date.slice(0, 10) <= q.get('to')!.slice(0, 10))
    .sort((a, b) => b.date.localeCompare(a.date))
}

function detail(c: DemoContext, g: Gig) {
  const l = links(c, g.id)
  return {
    ...g,
    linked_venues: l.venues.flatMap((v) => {
      const venue = c.state.venues.find((x) => x.id === v.venue_id)
      return venue ? [{ venue: { id: venue.id, name: venue.name }, is_primary: v.is_primary }] : []
    }),
    linked_contacts: l.contacts.flatMap((x) => {
      const contact = c.state.contacts.find((k) => k.id === x.contact_id)
      return contact ? [{ contact: { id: contact.id, name: contact.name }, role: x.role }] : []
    }),
    tracklists: l.tracklists.flatMap((id) => {
      const tl = c.state.tracklists.find((t) => t.id === id)
      return tl ? [{ id: tl.id, title: tl.title }] : []
    }),
  }
}

function bookingPdf(c: DemoContext, g: Gig): Blob {
  const dj = String(c.state.settings.dj_name || 'Sam Example')
  return pdfBlob([
    { text: 'Booking confirmation', size: 20, bold: true },
    { text: 'KlubHub DJ demo - fictional data, not a contract', size: 9 },
    { text: ' ' },
    { text: `Artist: ${dj}` },
    { text: `Event: ${g.event_name || '-'}` },
    { text: `Venue: ${[g.venue, g.city, g.country].filter(Boolean).join(', ')}` },
    { text: `Date: ${g.date.slice(0, 10)}` },
    { text: `Set length: ${g.set_length_minutes || '-'} minutes` },
    { text: `Fee: ${g.fee_amount} ${g.fee_currency}` },
    { text: `Promoter: ${g.promoter_name || '-'} ${g.promoter_email ? `<${g.promoter_email}>` : ''}` },
    { text: ' ' },
    { text: `Status: ${g.status} - payment: ${g.payment_status}` },
    ...(g.notes ? [{ text: ' ' }, { text: 'Notes', bold: true }, { text: g.notes }] : []),
  ])
}

function autocomplete<T>(items: T[], q: URLSearchParams, text: (t: T) => string): T[] {
  const term = (q.get('q') ?? '').toLowerCase()
  const limit = Math.min(Number(q.get('limit')) || 5, 20)
  if (term.length < 2) return []
  return items.filter((i) => text(i).toLowerCase().includes(term)).slice(0, limit)
}

export function registerGigs(r: DemoRouter): void {
  r.on('GET', '/api/v1/gigs', (req, c) => json(filterGigs(c.state.gigs, req.query)))
    .on('GET', '/api/v1/gigs/autocomplete', (req, c) => json(autocomplete(c.state.gigs, req.query, (g) => `${g.event_name} ${g.venue}`)))
    .on('GET', '/api/v1/gigs/calendar.ics', (_req, c) => ({
      status: 200,
      blob: new Blob([gigsIcs(c.state.gigs, c.now())], { type: 'text/calendar;charset=utf-8' }),
    }))

  r.on('POST', '/api/v1/gigs', (req, c) => {
    const b = body(req)
    if (!b.date) return apiError(400, 'date is required')
    const now = c.now().toISOString()
    const g: Gig = {
      id: uuid(), date: '', venue: '', city: '', country: '', event_name: '', promoter_name: '', promoter_email: '',
      promoter_phone: '', fee_amount: 0, fee_currency: 'EUR', set_length_minutes: 0, notes: '', status: 'inquiry',
      payment_status: 'unpaid', gig_reader_venue_id: null, gig_reader_contact_id: null, created_at: now, updated_at: now, deleted_at: null,
    }
    const err = apply(g, b)
    if (err) return apiError(400, err)
    c.state.gigs.unshift(g)
    return json(g, 201)
  })

  r.on('GET', '/api/v1/gigs/:id', (req, c) => {
    const g = c.state.gigs.find((x) => x.id === req.params.id)
    return g ? json(g) : notFound('gig not found')
  })
  r.on('GET', '/api/v1/gigs/:id/detail', (req, c) => {
    const g = c.state.gigs.find((x) => x.id === req.params.id)
    return g ? json(detail(c, g)) : notFound('gig not found')
  })

  r.on('PUT', '/api/v1/gigs/:id', (req, c) => {
    const g = c.state.gigs.find((x) => x.id === req.params.id)
    if (!g) return notFound('gig not found')
    const b = body(req)
    if (typeof b.updated_at !== 'string' || !b.updated_at) return apiError(400, 'updated_at is required')
    if (b.updated_at !== g.updated_at) return apiError(409, 'gig was updated elsewhere; reload and try again')
    const next = { ...g }
    const err = apply(next, b)
    if (err) return apiError(400, err)
    next.updated_at = c.now().toISOString()
    Object.assign(g, next)
    return json(g)
  })

  r.on('DELETE', '/api/v1/gigs/:id', (req, c) => {
    const before = c.state.gigs.length
    c.state.gigs = c.state.gigs.filter((g) => g.id !== req.params.id)
    Reflect.deleteProperty(c.state.gigLinks, req.params.id!)
    return before === c.state.gigs.length ? notFound('gig not found') : noContent()
  })

  r.on('GET', '/api/v1/gigs/:id/pdf', (req, c) => {
    const g = c.state.gigs.find((x) => x.id === req.params.id)
    return g ? { status: 200, blob: bookingPdf(c, g) } : notFound('gig not found')
  })

  r.on('POST', '/api/v1/gigs/:id/link-venue', (req, c) => {
    const b = body(req)
    if (!c.state.gigs.some((g) => g.id === req.params.id)) return notFound('gig not found')
    if (!c.state.venues.some((v) => v.id === b.venue_id)) return notFound('venue not found')
    const l = links(c, req.params.id!)
    if (b.is_primary) l.venues.forEach((v) => { v.is_primary = false })
    l.venues = [...l.venues.filter((v) => v.venue_id !== b.venue_id), { venue_id: String(b.venue_id), is_primary: !!b.is_primary }]
    return noContent()
  })

  r.on('POST', '/api/v1/gigs/:id/link-contact', (req, c) => {
    const b = body(req)
    if (!c.state.gigs.some((g) => g.id === req.params.id)) return notFound('gig not found')
    if (!c.state.contacts.some((k) => k.id === b.contact_id)) return notFound('contact not found')
    const l = links(c, req.params.id!)
    l.contacts = [...l.contacts.filter((k) => k.contact_id !== b.contact_id), { contact_id: String(b.contact_id), role: String(b.role ?? 'promoter') }]
    return noContent()
  })

  r.on('POST', '/api/v1/gigs/:id/tracklists/:tracklistId', (req, c) => {
    if (!c.state.gigs.some((g) => g.id === req.params.id)) return notFound('gig not found')
    if (!c.state.tracklists.some((t) => t.id === req.params.tracklistId)) return notFound('tracklist not found')
    const l = links(c, req.params.id!)
    if (!l.tracklists.includes(req.params.tracklistId!)) l.tracklists.push(req.params.tracklistId!)
    return noContent()
  })

  r.on('DELETE', '/api/v1/gigs/:id/tracklists/:tracklistId', (req, c) => {
    const l = links(c, req.params.id!)
    l.tracklists = l.tracklists.filter((t) => t !== req.params.tracklistId)
    return noContent()
  })

  registerDirectory(r, 'venues', (c) => c.state.venues, (v: Venue) => `${v.name} ${v.city}`)
  registerDirectory(r, 'contacts', (c) => c.state.contacts, (k: Contact) => `${k.name} ${k.company ?? ''} ${k.email}`)
}

/** List / autocomplete / get / create / update for venues and contacts. */
function registerDirectory<T extends { id: string; created_at: string; updated_at: string }>(
  r: DemoRouter, name: string, list: (c: DemoContext) => T[], text: (t: T) => string,
): void {
  const base = `/api/v1/${name}`
  r.on('GET', base, (req, c) => json(req.query.get('q') ? autocomplete(list(c), req.query, text) : list(c)))
    .on('GET', `${base}/autocomplete`, (req, c) => json(autocomplete(list(c), req.query, text)))
    .on('GET', `${base}/:id`, (req, c) => {
      const item = list(c).find((x) => x.id === req.params.id)
      return item ? json(item) : notFound(`${name.slice(0, -1)} not found`)
    })
    .on('POST', base, (req, c) => {
      const b = body(req)
      if (!String(b.name ?? '').trim()) return apiError(400, 'name is required')
      const now = c.now().toISOString()
      const item = { deleted_at: null, notes: '', ...b, id: uuid(), created_at: now, updated_at: now } as unknown as T
      list(c).push(item)
      return json(item, 201)
    })
    .on('PUT', `${base}/:id`, (req, c) => {
      const item = list(c).find((x) => x.id === req.params.id)
      if (!item) return notFound(`${name.slice(0, -1)} not found`)
      Object.assign(item, body(req), { id: item.id, created_at: item.created_at, updated_at: c.now().toISOString() })
      return json(item)
    })
}
