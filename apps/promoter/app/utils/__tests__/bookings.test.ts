import { describe, expect, it } from 'vitest'
import type { PromoterEvent } from '~/types/event'
import type { OpenSlot, TalentContact } from '~/types/bookings'
import {
  formatEventDate, formatFee, mentionsVenue, offerErrorText, offerStatusClass,
  offerVariables, previewVenue, renderOfferText, venueWithheld,
} from '../bookings'

const NOW = new Date('2026-10-15T12:00:00Z')

const event = (over: Partial<PromoterEvent>): PromoterEvent => ({
  id: 'e-1', title: 'Klubnacht 03', slug: 'klubnacht-03', status: 'published', visibility: 'public',
  publish_at: null, starts_at: '2026-10-21T21:00:00Z', ends_at: '2026-10-22T05:00:00Z', doors_at: null,
  timezone: 'Europe/Berlin', venue_id: 'v-1', city: 'Berlin', location_mode: 'venue', location_reveal_at: null,
  min_age: 18, genres: ['techno'], description_md: '', cost_text: '', external_ticket_url: null,
  capacity: 300, version: 1, created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z',
  ...over,
})

const contact: TalentContact = {
  id: 'tc-1', name: 'Ben Klock', email: 'ben@example.org', phone: '', has_email: true,
  city: 'Berlin', country: 'DE', genres: ['techno'], source: 'past_gig', notes: '',
  created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z',
}

const slot: OpenSlot = {
  id: 'os-1', event_id: 'e-1', role: 'Headliner', genres: ['techno', 'hard techno'],
  fee_min: 500, fee_max: 800, currency: 'EUR', notes: 'All-night set', created_at: '2026-10-01T10:00:00Z',
}

describe('venueWithheld (OFFER-09)', () => {
  it('withholds for city_only always', () => {
    expect(venueWithheld('city_only', null, NOW)).toBe(true)
    expect(venueWithheld('city_only', '2020-01-01T00:00:00Z', NOW)).toBe(true)
  })

  it('withholds for secret before the reveal, reveals after it', () => {
    expect(venueWithheld('secret', '2026-10-20T00:00:00Z', NOW)).toBe(true)
    expect(venueWithheld('secret', '2026-10-01T00:00:00Z', NOW)).toBe(false)
    // No reveal time set: withheld indefinitely (mirrors offerVenueName).
    expect(venueWithheld('secret', null, NOW)).toBe(true)
  })

  it('never withholds for venue mode', () => {
    expect(venueWithheld('venue', null, NOW)).toBe(false)
  })

  it('previewVenue renders empty when withheld, the name otherwise', () => {
    expect(previewVenue(event({ location_mode: 'city_only' }), 'Tresor.West', NOW)).toBe('')
    expect(previewVenue(event({ location_mode: 'secret', location_reveal_at: '2026-10-20T00:00:00Z' }), 'Tresor.West', NOW)).toBe('')
    expect(previewVenue(event({ location_mode: 'venue' }), 'Tresor.West', NOW)).toBe('Tresor.West')
    expect(previewVenue(event({ location_mode: 'venue' }), null, NOW)).toBe('')
  })
})

describe('formatFee', () => {
  it('renders the four shapes and the empty one', () => {
    expect(formatFee({ ...slot, fee_min: 500, fee_max: 500 })).toBe('500 EUR')
    expect(formatFee(slot)).toBe('500–800 EUR')
    expect(formatFee({ ...slot, fee_max: null })).toBe('from 500 EUR')
    expect(formatFee({ ...slot, fee_min: null })).toBe('up to 800 EUR')
    expect(formatFee({ ...slot, fee_min: null, fee_max: null })).toBe('')
    expect(formatFee(null)).toBe('')
  })
})

describe('formatEventDate', () => {
  it('renders the start in the event timezone', () => {
    // 21:00 UTC on 21 Oct is 23:00 in Berlin (CEST).
    expect(formatEventDate('2026-10-21T21:00:00Z', 'Europe/Berlin')).toBe('2026-10-21 23:00 CEST')
  })
})

describe('offerVariables + renderOfferText', () => {
  it('substitutes every variable and leaves unknown ones empty', () => {
    const vars = offerVariables({
      orgName: 'Nachtwerk', promoterName: 'Nina', promoterEmail: 'nina@nachtwerk.example',
      contact, event: event({}), venueName: 'Tresor.West', slot, now: NOW,
    })
    expect(vars.dj_name).toBe('Ben Klock')
    expect(vars.event_name).toBe('Klubnacht 03')
    expect(vars.city).toBe('Berlin')
    expect(vars.venue).toBe('Tresor.West')
    expect(vars.genres).toBe('techno, hard techno')
    expect(vars.fee).toBe('500–800 EUR')
    expect(vars.slot_role).toBe('Headliner')
    expect(vars.notes).toBe('All-night set')
    expect(vars.promoter_phone).toBe('')
    const out = renderOfferText('Hi {{dj_name}}, play {{event_name}} at {{venue}} — {{unknown}}', vars)
    expect(out).toBe('Hi Ben Klock, play Klubnacht 03 at Tresor.West — ')
  })

  it('renders the venue empty for a withheld event', () => {
    const vars = offerVariables({
      orgName: 'Nachtwerk', promoterName: 'Nina', promoterEmail: 'nina@nachtwerk.example',
      contact, event: event({ location_mode: 'secret', location_reveal_at: '2026-10-20T00:00:00Z' }),
      venueName: 'Tresor.West', slot, now: NOW,
    })
    expect(renderOfferText('Venue: {{venue}}', vars)).toBe('Venue: ')
  })

  it('works without event, slot or contact', () => {
    const vars = offerVariables({ orgName: 'Nachtwerk', promoterName: 'Nina', promoterEmail: 'n@x', contact: null, event: null, venueName: null, slot: null, now: NOW })
    expect(vars.dj_name).toBe('')
    expect(vars.event_name).toBeUndefined()
    expect(vars.fee).toBeUndefined()
  })
})

describe('mentionsVenue', () => {
  it('detects {{venue}} with any spacing', () => {
    expect(mentionsVenue('Venue: {{venue}}')).toBe(true)
    expect(mentionsVenue('Venue: {{ venue }}')).toBe(true)
    expect(mentionsVenue('City: {{city}}')).toBe(false)
  })
})

describe('offerErrorText', () => {
  it('maps every send error code to its copy', () => {
    expect(offerErrorText({ error: 'email_unavailable' })).toContain('not configured')
    expect(offerErrorText({ error: 'suppressed' })).toContain('do-not-contact')
    expect(offerErrorText({ error: 'conflict' })).toContain('changed')
    expect(offerErrorText({ error: 'rate_limited' })).toContain('Daily offer limit')
    expect(offerErrorText({ error: 'no_contact_email' })).toContain('no email address')
    expect(offerErrorText({ error: 'no_reply_to' })).toContain('reply address')
    expect(offerErrorText({ error: 'invalid', field: 'subject', problem: 'required' })).toBe('Invalid subject: required')
    expect(offerErrorText(null)).toBe('Could not send the offer.')
  })
})

describe('offerStatusClass', () => {
  it('maps statuses to HUD badge tones', () => {
    expect(offerStatusClass('sent')).toBe('badge-ready')
    expect(offerStatusClass('queued')).toBe('badge-draft')
    expect(offerStatusClass('sending')).toBe('badge-draft')
    expect(offerStatusClass('failed')).toBe('badge-failed')
    expect(offerStatusClass('bounced')).toBe('badge-failed')
  })
})

describe('offerErrorText — provider-terms gate', () => {
  it('explains an admin step for outreach_not_permitted', () => {
    const text = offerErrorText({ error: 'outreach_not_permitted' } as never)
    expect(text).toContain('OUTREACH_PROVIDER_PERMITS_OUTREACH')
  })

  it('leaves email_unavailable and the other reasons alone', () => {
    expect(offerErrorText({ error: 'email_unavailable' } as never))
      .toContain('Email is not configured')
    expect(offerErrorText({ error: 'suppressed' } as never))
      .toBe('This DJ is on your do-not-contact list.')
  })
})
