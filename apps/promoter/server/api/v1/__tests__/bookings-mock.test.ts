import { describe, expect, it, vi } from 'vitest'
import { createError } from 'h3'
import {
  addSuppression, createSlot, createTalent, createTemplate, deleteSlot, deleteSuppression,
  deleteTalent, deleteTemplate, getOffer, listOffers, listSlots, listSuppressions, listTalent,
  listTemplates, matchSlot, sendOffer, suppressTalent, updateTalent, updateTemplate,
} from '../-mockDb'

// -mockDb uses the Nitro auto-imports (createError); stub them for vitest.
vi.stubGlobal('createError', createError)

interface MockErr { statusCode?: number, data?: { error?: string, field?: string, problem?: string } }

function catchErr(fn: () => unknown): MockErr {
  try {
    fn()
  } catch (e) {
    return e as MockErr
  }
  throw new Error('expected the call to throw')
}

const input = {
  name: 'Test DJ', email: 'test-dj@example.org', phone: '', city: 'Berlin', country: 'DE',
  genres: ['techno'], source: 'manual' as const, notes: '',
}

describe('bookings mock (dev API mirror)', () => {
  it('lists talent newest-first with city, genre and q filters', () => {
    const all = listTalent()
    expect(all.length).toBeGreaterThanOrEqual(5)
    expect(all[0].id).toBe('tc-finn') // most recently added
    expect(listTalent({ city: 'hamburg' }).map(c => c.id)).toEqual(['tc-finn'])
    expect(listTalent({ genre: 'acid' }).map(c => c.id)).toEqual(['tc-lena'])
    expect(listTalent({ q: 'nadia' }).map(c => c.id)).toEqual(['tc-nadia'])
  })

  it('validates talent writes and rejects duplicate emails', () => {
    expect(catchErr(() => createTalent({ ...input, name: '', email: '' })).data?.error).toBe('invalid')
    expect(catchErr(() => createTalent({ ...input, email: 'not-an-email' })).data?.error).toBe('invalid')
    expect(catchErr(() => createTalent({ ...input, email: 'nadia@kovadjs.example' })).data?.error).toBe('duplicate')
    const c = createTalent(input)
    expect(c.has_email).toBe(true)
    const noEmail = createTalent({ ...input, name: 'No Mail', email: '', city: 'Berlin' })
    expect(noEmail.has_email).toBe(false)
    expect(catchErr(() => updateTalent('tc-missing', input)).statusCode).toBe(404)
    expect(catchErr(() => updateTalent(c.id, { ...input, email: 'omar@reyes.example' })).data?.error).toBe('duplicate')
    expect(catchErr(() => deleteTalent('tc-missing')).statusCode).toBe(404)
    deleteTalent(c.id)
    deleteTalent(noEmail.id)
  })

  it('lists slots per event and validates slot writes', () => {
    expect(listSlots('e-klubnacht').map(s => s.id)).toEqual(['os-klub-head', 'os-klub-open'])
    expect(catchErr(() => listSlots('e-missing')).statusCode).toBe(404)
    expect(catchErr(() => createSlot('e-klubnacht', { role: 'X', genres: [], fee_min: 800, fee_max: 500, currency: 'EUR', notes: '' })).data?.error).toBe('invalid')
    expect(catchErr(() => createSlot('e-klubnacht', { role: 'X', genres: [], fee_min: null, fee_max: null, currency: 'EURO', notes: '' })).data?.error).toBe('invalid')
    // UNIQUE (event_id, role): the seeded Headliner slot collides.
    expect(catchErr(() => createSlot('e-klubnacht', { role: 'Headliner', genres: [], fee_min: null, fee_max: null, currency: null, notes: '' })).data?.error).toBe('conflict')
    const s = createSlot('e-klubnacht', { role: 'B2B', genres: ['techno'], fee_min: 300, fee_max: null, currency: 'EUR', notes: '' })
    expect(s.id).toMatch(/^os-/)
    deleteSlot(s.id)
    expect(catchErr(() => deleteSlot(s.id)).statusCode).toBe(404)
  })

  it('matches a slot by city + genre overlap, ranked, excluding suppressed', () => {
    // os-klub-head: e-klubnacht (Berlin, techno). Finn (Hamburg) never matches.
    const head = matchSlot('os-klub-head')
    expect(head.map(m => m.contact.id)).toEqual(['tc-lena', 'tc-june', 'tc-omar', 'tc-nadia'])
    expect(head.every(m => m.overlap_genres.join() === 'techno')).toBe(true)
    // The no-email contact still matches (OFFER-10: matchable, not sendable).
    expect(head.find(m => m.contact.id === 'tc-lena')?.contact.has_email).toBe(false)
    // Matching uses the EVENT's genres (e-klubnacht: techno), not the slot's —
    // so the genre-less slot matches the same ranked set.
    const open = matchSlot('os-klub-open')
    expect(open.map(m => m.contact.id)).toEqual(['tc-lena', 'tc-june', 'tc-omar', 'tc-nadia'])
    expect(open.every(m => m.overlap_genres.join() === 'techno')).toBe(true)
    expect(catchErr(() => matchSlot('os-missing')).statusCode).toBe(404)
  })

  it('runs the suppression flow (one-click opt-out)', () => {
    // A contact without an email cannot be suppressed.
    expect(catchErr(() => suppressTalent('tc-lena')).data?.error).toBe('no_contact_email')
    const s = suppressTalent('tc-june')
    expect(s.reason).toBe('marked do-not-contact')
    expect('email' in s).toBe(false) // blind index only — the API never returns it
    expect(matchSlot('os-klub-head').map(m => m.contact.id)).not.toContain('tc-june')
    expect(catchErr(() => addSuppression({ email: 'june@park.example', reason: 'again' })).data?.error).toBe('duplicate')
    expect(catchErr(() => addSuppression({ email: '', reason: 'x' })).data?.error).toBe('invalid')
    expect(listSuppressions().every(x => !('email' in x))).toBe(true)
    deleteSuppression(s.id)
    expect(matchSlot('os-klub-head').map(m => m.contact.id)).toContain('tc-june')
  })

  it('runs template CRUD with optimistic concurrency', () => {
    expect(catchErr(() => createTemplate({ name: '', subject: 's', body: 'b', is_default: false })).data?.error).toBe('invalid')
    expect(catchErr(() => createTemplate({ name: 'Default booking offer', subject: 's', body: 'b', is_default: false })).data?.error).toBe('conflict')
    const t = createTemplate({ name: 'Mock template', subject: 'Hi {{dj_name}}', body: 'Body', is_default: true })
    expect(t.version).toBe(1)
    // Setting a default clears the others.
    expect(listTemplates().find(x => x.id === 'tpl-default')?.is_default).toBe(false)
    expect(catchErr(() => updateTemplate(t.id, { version: 99 })).data?.error).toBe('conflict')
    expect(catchErr(() => updateTemplate('tpl-missing', { version: 1 })).statusCode).toBe(404)
    const u = updateTemplate(t.id, { subject: 'Hi {{dj_name}}!', version: 1 })
    expect(u.version).toBe(2)
    expect(u.subject).toBe('Hi {{dj_name}}!')
    deleteTemplate(t.id)
    expect(catchErr(() => deleteTemplate(t.id)).statusCode).toBe(404)
  })

  it('sends an offer: renders variables, withholds the venue (OFFER-09)', () => {
    expect(catchErr(() => sendOffer({ talent_contact_id: '' })).data?.error).toBe('invalid')
    expect(catchErr(() => sendOffer({ talent_contact_id: 'tc-missing' })).statusCode).toBe(404)
    expect(catchErr(() => sendOffer({ talent_contact_id: 'tc-lena' })).data?.error).toBe('no_contact_email')
    expect(catchErr(() => sendOffer({ talent_contact_id: 'tc-nadia', open_slot_id: 'os-missing' })).data?.error).toBe('forbidden')
    expect(catchErr(() => sendOffer({ talent_contact_id: 'tc-nadia', template_id: 'tpl-missing' })).statusCode).toBe(404)

    // Venue event: {{venue}} renders the venue name.
    const sent = sendOffer({ talent_contact_id: 'tc-nadia', open_slot_id: 'os-klub-head' })
    expect(sent.status).toBe('sent')
    expect(sent.to_email).toBe('nadia@kovadjs.example')
    expect(sent.subject).toContain('Klubnacht 03')
    expect(sent.body).toContain('Venue: Tresor.West')
    expect(sent.body).toContain('Hi Nadia Kova,')
    expect(sent.body).not.toContain('{{')

    // Secret event before the reveal: {{venue}} renders empty.
    const secret = sendOffer({ talent_contact_id: 'tc-omar', open_slot_id: 'os-ware' })
    expect(secret.body).toContain('Venue: \n')
    expect(secret.body).not.toContain('Tresor.West')

    // Per-request overrides replace the template copy before rendering.
    const custom = sendOffer({ talent_contact_id: 'tc-omar', open_slot_id: 'os-ware', subject: 'Custom {{event_name}}', body: 'Custom body' })
    expect(custom.subject).toBe('Custom Warehouse 10')
    expect(custom.body).toBe('Custom body')

    expect(getOffer(sent.id).id).toBe(sent.id)
    expect(catchErr(() => getOffer('or-missing')).statusCode).toBe(404)
    // The sent log is newest first.
    expect(listOffers()[0].id).toBe(custom.id)
  })

  it('refuses a suppressed recipient', () => {
    const s = addSuppression({ email: 'june@park.example', reason: 'test opt-out' })
    expect(catchErr(() => sendOffer({ talent_contact_id: 'tc-june' })).data?.error).toBe('suppressed')
    deleteSuppression(s.id)
  })

  it('enforces the daily outreach cap (429 rate_limited)', () => {
    // The cap counts today's requests; earlier tests in this file sent some.
    const today = () => listOffers()
      .filter(r => new Date(r.created_at).toDateString() === new Date().toDateString()).length
    for (let i = today(); i < 50; i++) sendOffer({ talent_contact_id: 'tc-nadia' })
    const err = catchErr(() => sendOffer({ talent_contact_id: 'tc-nadia' }))
    expect(err.statusCode).toBe(429)
    expect(err.data?.error).toBe('rate_limited')
  })
})
