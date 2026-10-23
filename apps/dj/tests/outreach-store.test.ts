// The outreach store (04.10-04) against a stubbed $fetch: every action calls
// the right route with the right body and updates state in place; sendRequest
// prepends to the sent log; addSuppression dedupes; failures land on the
// section's error.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useOutreachStore } from '../app/stores/outreach'
import type {
  AvailabilitySlot,
  MatchedContact,
  OutreachRequest,
  OutreachTemplate,
  SlotMatch,
  Suppression,
} from '../app/types/outreach'

const fetchMock = vi.fn()
const httpError = (statusCode: number, body: unknown) =>
  Object.assign(new Error(`HTTP ${statusCode}`), { statusCode, data: body })

const slot = (over: Partial<AvailabilitySlot> = {}): AvailabilitySlot => ({
  id: 'os-1', date: '2026-11-06', city: 'Berlin', country: 'DE', genres: ['techno'],
  fee_min: 800, fee_max: 1200, currency: 'EUR', notes: '', created_at: '2026-10-01T10:00:00Z', ...over,
})
const tpl = (over: Partial<OutreachTemplate> = {}): OutreachTemplate => ({
  id: 'tpl-1', name: 'Default booking request', direction: 'dj_to_promoter',
  subject: 'Booking request: {{dj_name}} in {{city}} on {{date}}', body: 'Hi {{promoter_name}}', is_default: true,
  version: 1, created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z', ...over,
})
const req = (over: Partial<OutreachRequest> = {}): OutreachRequest => ({
  id: 'or-1', contact_id: 'c-1', availability_slot_id: 'os-1', template_id: 'tpl-1', epk_export_id: null,
  email_message_id: 'em-1', to_email: 'bookings@nachtwerk.example', subject: 'Booking request', body: 'Hi',
  status: 'sent', sent_at: '2026-10-02T10:00:00Z', created_at: '2026-10-02T10:00:00Z', email_status: 'delivered', ...over,
})
const sup = (over: Partial<Suppression> = {}): Suppression => ({
  id: 'sup-1', email: 'stopme@example.com', reason: 'replied "stop"', created_at: '2026-10-01T10:00:00Z', ...over,
})
const contact: MatchedContact = {
  id: 'c-1', name: 'Nachtwerk Bookings', company: 'Nachtwerk', email: 'bookings@nachtwerk.example',
  has_email: true, city: 'Berlin', past_gig_in_city: true,
}
const match = (over: Partial<SlotMatch> = {}): SlotMatch => ({
  slot: slot(), contacts: [contact], venues: [{ id: 'v-1', name: 'Tresor.West', city: 'Berlin' }], ...over,
})

beforeEach(() => {
  setActivePinia(createPinia())
  fetchMock.mockReset()
  vi.stubGlobal('$fetch', fetchMock)
})

describe('outreach store', () => {
  it('loads every section from its route', async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url === '/api/v1/outreach/availability/slots') return Promise.resolve([slot()])
      if (url === '/api/v1/outreach/templates') return Promise.resolve([tpl()])
      if (url === '/api/v1/outreach/requests') return Promise.resolve([req()])
      if (url === '/api/v1/outreach/suppressions') return Promise.resolve([sup()])
      if (url === '/api/v1/epk/exports') return Promise.resolve({ data: [{ id: 'ex-1', garageObjectKey: 'epk/exports/1.pdf', createdAt: '2026-10-01T10:00:00Z' }] })
      return Promise.reject(new Error(`unexpected ${url}`))
    })
    const store = useOutreachStore()
    await store.loadSlots()
    await store.loadTemplates()
    await store.loadRequests()
    await store.loadSuppressions()
    await store.loadEpkExports()
    expect(store.slots).toHaveLength(1)
    expect(store.templates).toHaveLength(1)
    expect(store.requests).toHaveLength(1)
    expect(store.suppressions).toHaveLength(1)
    expect(store.epkExports[0]?.garageObjectKey).toBe('epk/exports/1.pdf')
    expect(store.slotsSec.error).toBeNull()
  })

  it('adds a slot and removes it (also dropping its matches)', async () => {
    const store = useOutreachStore()
    store.slots = [slot()]
    store.matches = [match()]
    store.selectedSlotId = 'os-1'
    fetchMock.mockResolvedValue(slot({ id: 'os-2', city: 'Hamburg' }))
    const created = await store.addSlot({
      date: '2026-12-01', city: 'Hamburg', country: 'DE', genres: ['house'],
      fee_min: null, fee_max: null, currency: null, notes: '',
    })
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/outreach/availability/slots')
    expect(opts.method).toBe('POST')
    expect(opts.body).toMatchObject({ city: 'Hamburg', genres: ['house'] })
    expect(created.id).toBe('os-2')
    expect(store.slots.map(s => s.id)).toEqual(['os-1', 'os-2'])

    fetchMock.mockResolvedValue(null)
    await store.removeSlot('os-1')
    expect(fetchMock.mock.calls[1]![0]).toBe('/api/v1/outreach/availability/slots/os-1')
    expect(fetchMock.mock.calls[1]![1].method).toBe('DELETE')
    expect(store.slots.map(s => s.id)).toEqual(['os-2'])
    expect(store.matches).toHaveLength(0)
    expect(store.selectedSlotId).toBeNull()
  })

  it('loads matches for the selected slot and clears them', async () => {
    fetchMock.mockResolvedValue([match()])
    const store = useOutreachStore()
    await store.loadMatches('os-1')
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/outreach/availability/matches')
    expect(fetchMock.mock.calls[0]![1].query).toEqual({ slot_id: 'os-1' })
    expect(store.selectedSlotId).toBe('os-1')
    expect(store.matches[0]?.contacts[0]?.name).toBe('Nachtwerk Bookings')

    store.clearMatches()
    expect(store.matches).toHaveLength(0)
    expect(store.selectedSlotId).toBeNull()
  })

  it('creates and updates templates (version bump lands in place)', async () => {
    const store = useOutreachStore()
    store.templates = [tpl()]
    fetchMock.mockResolvedValue(tpl({ id: 'tpl-2', name: 'Short ask', version: 1 }))
    await store.saveTemplate({ name: 'Short ask', subject: 'Hi', body: 'Body', is_default: false })
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/outreach/templates')
    expect(store.templates.map(t => t.id)).toEqual(['tpl-1', 'tpl-2'])

    fetchMock.mockResolvedValue(tpl({ version: 2, subject: 'Hi!' }))
    await store.updateTemplate('tpl-1', { subject: 'Hi!', version: 1 })
    const [url, opts] = fetchMock.mock.calls[1]!
    expect(url).toBe('/api/v1/outreach/templates/tpl-1')
    expect(opts.method).toBe('PUT')
    expect(opts.body).toMatchObject({ subject: 'Hi!', version: 1 })
    expect(store.templates[0]?.version).toBe(2)

    fetchMock.mockResolvedValue(null)
    await store.removeTemplate('tpl-2')
    expect(store.templates.map(t => t.id)).toEqual(['tpl-1'])
  })

  it('sendRequest posts the body and prepends the request to the sent log', async () => {
    const store = useOutreachStore()
    store.requests = [req({ id: 'or-0' })]
    fetchMock.mockResolvedValue(req({ id: 'or-1' }))
    const sent = await store.sendRequest({
      contact_id: 'c-1', availability_slot_id: 'os-1', template_id: 'tpl-1',
      epk_export_id: 'ex-1', subject: null, body: null,
    })
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/outreach/requests')
    expect(opts.method).toBe('POST')
    expect(opts.body).toMatchObject({ contact_id: 'c-1', epk_export_id: 'ex-1' })
    expect(sent.id).toBe('or-1')
    expect(store.requests.map(r => r.id)).toEqual(['or-1', 'or-0'])
  })

  it('addSuppression prepends once (the API is idempotent on duplicates)', async () => {
    const store = useOutreachStore()
    store.suppressions = [sup()]
    fetchMock.mockResolvedValue(sup())
    await store.addSuppression({ email: 'STOPME@example.com', reason: 'opted out via the sent log' })
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/outreach/suppressions')
    expect(store.suppressions).toHaveLength(1)

    fetchMock.mockResolvedValue(sup({ id: 'sup-2', email: 'other@example.com' }))
    await store.addSuppression({ email: 'other@example.com', reason: 'opted out' })
    expect(store.suppressions.map(s => s.id)).toEqual(['sup-2', 'sup-1'])

    fetchMock.mockResolvedValue(null)
    await store.removeSuppression('sup-1')
    expect(store.suppressions.map(s => s.id)).toEqual(['sup-2'])
  })

  it('records failures on the section and rethrows', async () => {
    fetchMock.mockRejectedValue(httpError(503, { error: 'email is not available' }))
    const store = useOutreachStore()
    await expect(store.sendRequest({ contact_id: 'c-1' })).rejects.toMatchObject({ statusCode: 503 })
    expect(store.requestsSec.error).toMatchObject({ statusCode: 503 })
    expect(store.requestsSec.loading).toBe(false)
    expect(store.requests).toHaveLength(0)
  })
})
