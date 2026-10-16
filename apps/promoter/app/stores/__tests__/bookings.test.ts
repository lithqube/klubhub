import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useBookingsStore } from '../bookings'
import type { OpenSlot, OutreachRequest, OutreachTemplate, TalentContact, TalentMatch } from '~/types/bookings'

const fetchMock = vi.fn()
vi.stubGlobal('$fetch', fetchMock)

const contact: TalentContact = {
  id: 'tc-1', name: 'Ben Klock', email: 'ben@example.org', phone: '', has_email: true,
  city: 'Berlin', country: 'DE', genres: ['techno'], source: 'past_gig', notes: '',
  created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z',
}

const slot: OpenSlot = {
  id: 'os-1', event_id: 'e-1', role: 'Headliner', genres: ['techno'],
  fee_min: 500, fee_max: 800, currency: 'EUR', notes: '', created_at: '2026-10-01T10:00:00Z',
}

const template: OutreachTemplate = {
  id: 'ot-1', name: 'Default booking offer', direction: 'promoter_to_dj',
  subject: 'Booking offer: {{event_name}} on {{date}}', body: 'Hi {{dj_name}},', is_default: true, version: 1,
  created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z',
}

const offer: OutreachRequest = {
  id: 'or-1', event_id: 'e-1', open_slot_id: 'os-1', talent_contact_id: 'tc-1', template_id: 'ot-1',
  email_message_id: 'em-1', to_email: 'ben@example.org', subject: 'Booking offer', body: 'Hi Ben,',
  status: 'sent', sent_at: '2026-10-02T10:00:00Z', created_at: '2026-10-02T10:00:00Z',
}

/** Routes the stub by URL + method, like the API's route table. */
function stubRoutes(handlers: Record<string, (url: string, opts?: { method?: string, body?: unknown }) => unknown>) {
  fetchMock.mockImplementation((url: string, opts?: { method?: string }) => {
    const method = opts?.method ?? 'GET'
    for (const [key, fn] of Object.entries(handlers)) {
      const [m, path] = key.split(' ')
      if (m === method && url === path) return Promise.resolve(fn(url, opts as { method?: string, body?: unknown }))
    }
    return Promise.reject(Object.assign(new Error(`unstubbed ${method} ${url}`), { statusCode: 404 }))
  })
}

describe('useBookingsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
  })

  it('loads talent, templates, offers and suppressions', async () => {
    stubRoutes({
      'GET /api/v1/bookings/talent': () => [contact],
      'GET /api/v1/bookings/templates': () => [template],
      'GET /api/v1/bookings/offers': () => [offer],
      'GET /api/v1/bookings/suppressions': () => [],
    })
    const s = useBookingsStore()
    await Promise.all([s.loadTalent(), s.loadTemplates(), s.loadOffers(), s.loadSuppressions()])
    expect(s.talent.map(c => c.id)).toEqual(['tc-1'])
    expect(s.templates.map(t => t.id)).toEqual(['ot-1'])
    expect(s.offers.map(o => o.id)).toEqual(['or-1'])
    expect(s.suppressions).toEqual([])
    expect(s.talentSec.error).toBeNull()
  })

  it('passes the talent filter as query params', async () => {
    stubRoutes({ 'GET /api/v1/bookings/talent': () => [] })
    const s = useBookingsStore()
    await s.loadTalent({ city: 'Berlin', genre: 'techno', q: 'ben' })
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/bookings/talent')
    expect(opts.query).toEqual({ city: 'Berlin', genre: 'techno', q: 'ben' })
  })

  it('creates a talent contact and prepends it', async () => {
    stubRoutes({
      'POST /api/v1/bookings/talent': (_u, o) => ({ ...contact, id: 'tc-2', name: (o?.body as { name: string }).name }),
    })
    const s = useBookingsStore()
    const c = await s.createTalent({ name: 'New DJ', email: '', phone: '', city: '', country: '', genres: [], source: 'manual', notes: '' })
    expect(c.id).toBe('tc-2')
    expect(s.talent[0]!.id).toBe('tc-2')
  })

  it('updates a talent contact in place', async () => {
    stubRoutes({
      'GET /api/v1/bookings/talent': () => [contact],
      'PUT /api/v1/bookings/talent/tc-1': () => ({ ...contact, city: 'Lisbon' }),
    })
    const s = useBookingsStore()
    await s.loadTalent()
    await s.updateTalent('tc-1', { name: 'Ben Klock', email: 'ben@example.org', phone: '', city: 'Lisbon', country: 'PT', genres: ['techno'], source: 'past_gig', notes: '' })
    expect(s.talent[0]!.city).toBe('Lisbon')
  })

  it('deletes a talent contact', async () => {
    stubRoutes({
      'GET /api/v1/bookings/talent': () => [contact],
      'DELETE /api/v1/bookings/talent/tc-1': () => undefined,
    })
    const s = useBookingsStore()
    await s.loadTalent()
    await s.deleteTalent('tc-1')
    expect(s.talent).toHaveLength(0)
  })

  it('suppressing a contact removes it from talent and matches and logs the suppression', async () => {
    const match: TalentMatch = { contact, overlap_genres: ['techno'] }
    stubRoutes({
      'GET /api/v1/bookings/talent': () => [contact],
      'GET /api/v1/bookings/slots/os-1/matches': () => [match],
      'POST /api/v1/bookings/talent/tc-1/suppress': () => ({ id: 'sup-1', reason: '', created_at: '2026-10-03T10:00:00Z' }),
    })
    const s = useBookingsStore()
    await s.loadTalent()
    await s.loadMatches('os-1')
    const sup = await s.suppressTalent('tc-1')
    expect(sup.id).toBe('sup-1')
    expect(s.talent).toHaveLength(0)
    expect(s.matches).toHaveLength(0)
    expect(s.suppressions[0]!.id).toBe('sup-1')
  })

  it('loads slots for an event and creates one', async () => {
    stubRoutes({
      'GET /api/v1/bookings/events/e-1/slots': () => [slot],
      'POST /api/v1/bookings/events/e-1/slots': () => ({ ...slot, id: 'os-2', role: 'Opener' }),
    })
    const s = useBookingsStore()
    await s.loadSlots('e-1')
    expect(s.slots.map(x => x.id)).toEqual(['os-1'])
    await s.createSlot('e-1', { role: 'Opener', genres: ['techno'], fee_min: null, fee_max: null, currency: null, notes: '' })
    expect(s.slots.map(x => x.id)).toEqual(['os-1', 'os-2'])
  })

  it('deleting the selected slot clears the selection', async () => {
    stubRoutes({
      'GET /api/v1/bookings/events/e-1/slots': () => [slot],
      'DELETE /api/v1/bookings/slots/os-1': () => undefined,
    })
    const s = useBookingsStore()
    await s.loadSlots('e-1')
    s.selectedSlotId = 'os-1'
    await s.deleteSlot('os-1')
    expect(s.slots).toHaveLength(0)
    expect(s.selectedSlotId).toBeNull()
  })

  it('loadMatches selects the slot and stores the matches', async () => {
    const match: TalentMatch = { contact, overlap_genres: ['techno'] }
    stubRoutes({ 'GET /api/v1/bookings/slots/os-1/matches': () => [match] })
    const s = useBookingsStore()
    await s.loadMatches('os-1')
    expect(s.selectedSlotId).toBe('os-1')
    expect(s.matches[0]!.overlap_genres).toEqual(['techno'])
  })

  it('sendOffer posts the body and prepends the offer to the sent log', async () => {
    stubRoutes({
      'GET /api/v1/bookings/offers': () => [],
      'POST /api/v1/bookings/offers': (_u, o) => ({ ...offer, id: 'or-2', ...(o?.body as object) }),
    })
    const s = useBookingsStore()
    await s.loadOffers()
    const sent = await s.sendOffer({ talent_contact_id: 'tc-1', open_slot_id: 'os-1', template_id: 'ot-1', subject: 'Hi', body: 'Body' })
    expect(sent.id).toBe('or-2')
    expect(s.offers[0]!.id).toBe('or-2')
    const [, opts] = fetchMock.mock.calls.find(c => c[0] === '/api/v1/bookings/offers' && c[1]?.method === 'POST')!
    expect(opts.body).toEqual({ talent_contact_id: 'tc-1', open_slot_id: 'os-1', template_id: 'ot-1', subject: 'Hi', body: 'Body' })
  })

  it('template CRUD keeps the list in sync, update carries the version', async () => {
    stubRoutes({
      'GET /api/v1/bookings/templates': () => [template],
      'POST /api/v1/bookings/templates': () => ({ ...template, id: 'ot-2', is_default: false }),
      'PUT /api/v1/bookings/templates/ot-1': (_u, o) => ({ ...template, version: (o?.body as { version: number }).version + 1 }),
      'DELETE /api/v1/bookings/templates/ot-2': () => undefined,
    })
    const s = useBookingsStore()
    await s.loadTemplates()
    await s.createTemplate({ name: 'Follow-up', subject: 's', body: 'b', is_default: false })
    expect(s.templates.map(t => t.id)).toEqual(['ot-1', 'ot-2'])
    await s.updateTemplate('ot-1', { version: 1 })
    expect(s.templates[0]!.version).toBe(2)
    await s.deleteTemplate('ot-2')
    expect(s.templates.map(t => t.id)).toEqual(['ot-1'])
  })

  it('adds and deletes suppressions', async () => {
    stubRoutes({
      'GET /api/v1/bookings/suppressions': () => [],
      'POST /api/v1/bookings/suppressions': () => ({ id: 'sup-1', reason: 'stop', created_at: '2026-10-03T10:00:00Z' }),
      'DELETE /api/v1/bookings/suppressions/sup-1': () => undefined,
    })
    const s = useBookingsStore()
    await s.loadSuppressions()
    await s.addSuppression({ email: 'ben@example.org', reason: 'stop' })
    expect(s.suppressions[0]!.id).toBe('sup-1')
    await s.deleteSuppression('sup-1')
    expect(s.suppressions).toHaveLength(0)
  })

  it('surfaces API errors on the section and rethrows them normalised', async () => {
    fetchMock.mockRejectedValue(Object.assign(new Error('conflict'), { statusCode: 409, data: { error: 'suppressed' } }))
    const s = useBookingsStore()
    await expect(s.sendOffer({ talent_contact_id: 'tc-1' })).rejects.toMatchObject({ error: 'suppressed', status: 409 })
    expect(s.offersSec.error?.error).toBe('suppressed')
    expect(s.offersSec.loading).toBe(false)
  })
})
