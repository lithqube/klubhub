import { describe, expect, it } from 'vitest'
import {
  deliveryLabel,
  formatMoneyAmount,
  formatSlotDate,
  formatSlotFee,
  genresToText,
  renderRequestText,
  requestErrorText,
  requestStatusClass,
  requestVariables,
  textToGenres,
} from '../outreach'
import type { AvailabilitySlot, MatchedContact } from '../../types/outreach'

const slot = (over: Partial<AvailabilitySlot> = {}): AvailabilitySlot => ({
  id: 'os-1',
  date: '2026-11-06',
  city: 'Berlin',
  country: 'DE',
  genres: ['techno', 'hard techno'],
  fee_min: 800,
  fee_max: 1200,
  currency: 'EUR',
  notes: '23:00–02:00',
  created_at: '2026-10-01T10:00:00Z',
  ...over,
})

const contact = (over: Partial<MatchedContact> = {}): MatchedContact => ({
  id: 'c-1',
  name: 'Nachtwerk Bookings',
  company: 'Nachtwerk',
  email: 'bookings@nachtwerk.example',
  has_email: true,
  city: 'Berlin',
  past_gig_in_city: true,
  ...over,
})

describe('formatMoneyAmount', () => {
  it('trims trailing zeros like the Go formatMoney', () => {
    expect(formatMoneyAmount(800)).toBe('800')
    expect(formatMoneyAmount(800.5)).toBe('800.5')
    expect(formatMoneyAmount(0)).toBe('0')
  })
})

describe('formatSlotFee', () => {
  it('renders the fee range shapes', () => {
    expect(formatSlotFee(slot())).toBe('800–1200 EUR')
    expect(formatSlotFee(slot({ fee_min: 400, fee_max: 400 }))).toBe('400 EUR')
    expect(formatSlotFee(slot({ fee_max: null }))).toBe('from 800 EUR')
    expect(formatSlotFee(slot({ fee_min: null }))).toBe('up to 1200 EUR')
    expect(formatSlotFee(slot({ fee_min: null, fee_max: null }))).toBe('')
    expect(formatSlotFee(null)).toBe('')
  })
})

describe('requestVariables + renderRequestText', () => {
  it('builds the SPEC §7 variable map and renders the preview', () => {
    const vars = requestVariables({
      djName: 'SAMPLE_RATE',
      djEmail: 'sam@example.com',
      djPhone: '+49 30 555 0100',
      contact: contact(),
      slot: slot(),
      epkLink: 'https://epk.example/sam',
    })
    expect(vars.dj_name).toBe('SAMPLE_RATE')
    expect(vars.promoter_name).toBe('Nachtwerk Bookings')
    expect(vars.promoter_company).toBe('Nachtwerk')
    expect(vars.city).toBe('Berlin')
    expect(vars.date).toBe('2026-11-06')
    expect(vars.genres).toBe('techno, hard techno')
    expect(vars.fee).toBe('800–1200 EUR')
    expect(vars.epk_link).toBe('https://epk.example/sam')

    const out = renderRequestText('Hi {{promoter_name}}, I\'m {{dj_name}} in {{city}} on {{date}}. {{unknown}}', vars)
    expect(out).toBe('Hi Nachtwerk Bookings, I\'m SAMPLE_RATE in Berlin on 2026-11-06. ')
  })

  it('renders empty slot variables when no slot is attached', () => {
    const vars = requestVariables({
      djName: 'SAMPLE_RATE', djEmail: '', djPhone: '', contact: null, slot: null, epkLink: '',
    })
    expect(vars.city).toBeUndefined()
    expect(renderRequestText('{{city}}', vars)).toBe('')
  })
})

describe('requestErrorText', () => {
  const err = (statusCode: number, error?: string, message?: string) => ({ statusCode, data: { error, message } })

  it('maps the 503/409/429 surfaces', () => {
    expect(requestErrorText(err(503))).toContain('Email is not configured')
    expect(requestErrorText(err(409, 'suppressed'))).toContain('do-not-contact')
    expect(requestErrorText(err(409, 'conflict'))).toContain('template changed')
    expect(requestErrorText(err(429))).toContain('Daily outreach limit')
  })

  it('maps the 422 reasons (OUTR-05/06 v2.1) — the DJ API puts the text in data.error', () => {
    expect(requestErrorText(err(422, 'contact has no email address'))).toContain('no email address')
    expect(requestErrorText(err(422, 'no DJ contact email configured'))).toContain('Set your contact email')
    expect(requestErrorText(err(422, 'epk export has no garage object key'))).toContain('no file')
    expect(requestErrorText(err(422, 'validation failed: city: required'))).toBe('validation failed: city: required')
  })

  it('falls back to the message, then a generic line', () => {
    expect(requestErrorText(err(500, undefined, 'boom'))).toBe('boom')
    expect(requestErrorText(err(500, 'weird'))).toBe('weird')
    expect(requestErrorText(null)).toBe('Could not send the request.')
  })
})

describe('requestStatusClass + deliveryLabel', () => {
  it('maps statuses to HUD badge tones', () => {
    expect(requestStatusClass('sent')).toBe('badge-ready')
    expect(requestStatusClass('queued')).toBe('badge-draft')
    expect(requestStatusClass('sending')).toBe('badge-draft')
    expect(requestStatusClass('failed')).toBe('badge-failed')
    expect(requestStatusClass('bounced')).toBe('badge-failed')
  })

  it('prefers the outbox status for the delivery label', () => {
    expect(deliveryLabel({ status: 'sent', email_status: 'delivered' } as never)).toBe('DELIVERED')
    expect(deliveryLabel({ status: 'failed' } as never)).toBe('FAILED')
  })
})

describe('slot helpers', () => {
  it('formats the calendar date', () => {
    expect(formatSlotDate('2026-11-06')).toBe('Fri 2026-11-06')
    expect(formatSlotDate('not-a-date')).toBe('not-a-date')
  })

  it('round-trips the genres text input', () => {
    expect(textToGenres('techno, hard techno ,, ')).toEqual(['techno', 'hard techno'])
    expect(genresToText(['techno'])).toBe('techno')
  })
})

describe('requestErrorText — provider-terms gate', () => {
  it('explains an admin step for the outreach provider gate instead of echoing the raw sentence', () => {
    const text = requestErrorText({
      statusCode: 422,
      data: { error: 'outreach sending is disabled: your email provider\'s terms prohibit unsolicited B2B outreach' },
    } as never)
    expect(text).toContain('OUTREACH_PROVIDER_PERMITS_OUTREACH')
    expect(text).not.toContain('your email provider\'s terms prohibit')
  })

  it('recognises the coded error even without the long server sentence', () => {
    const text = requestErrorText({ statusCode: 422, data: { error: 'outreach_not_permitted' } } as never)
    expect(text).toContain('OUTREACH_PROVIDER_PERMITS_OUTREACH')
  })

  it('does not hijack the ordinary 422 reasons', () => {
    expect(requestErrorText({ statusCode: 422, data: { error: 'contact has no email address' } } as never))
      .toBe('This contact has no email address on file.')
  })
})
