// @vitest-environment node
// Contract rules enforced by the in-memory outreach mocks
// (apps/dj/server/api/v1/outreach/**, apps/dj/server/api/v1/epk/exports.get.ts).
// The rules mirror api/internal/outreach; the Nitro auto-imports are stubbed
// so the route handlers run as plain functions.
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AvailabilitySlot, OutreachRequest, OutreachTemplate, SlotMatch, Suppression } from '../app/types/outreach'

interface FakeEvent {
  params: Record<string, string>
  body?: unknown
  query?: Record<string, string>
  status: number
}

vi.stubGlobal('defineEventHandler', (h: unknown) => h)
vi.stubGlobal('setResponseStatus', (e: FakeEvent, s: number) => { e.status = s })
vi.stubGlobal('getRouterParam', (e: FakeEvent, name: string) => e.params[name])
vi.stubGlobal('readBody', async (e: FakeEvent) => e.body)
vi.stubGlobal('getQuery', (e: FakeEvent) => e.query ?? {})

type Handler = (e: FakeEvent) => unknown
const base = '../server/api/v1/outreach'
let db: typeof import('../server/api/v1/outreach/-mockDb')
const h: Record<string, Handler> = {}

beforeAll(async () => {
  db = await import(`${base}/-mockDb`)
  const load = async (key: string, path: string) => { h[key] = (await import(`${base}/${path}`)).default as Handler }
  await load('slotsList', 'availability/slots.get')
  await load('slotCreate', 'availability/slots.post')
  await load('slotDelete', 'availability/slots/[id].delete')
  await load('matches', 'availability/matches.get')
  await load('templatesList', 'templates.get')
  await load('templateCreate', 'templates.post')
  await load('templatePut', 'templates/[id].put')
  await load('templateDelete', 'templates/[id].delete')
  await load('requestsList', 'requests.get')
  await load('requestSend', 'requests.post')
  await load('requestGet', 'requests/[id].get')
  await load('suppressionsList', 'suppressions.get')
  await load('suppressionAdd', 'suppressions.post')
  await load('suppressionDelete', 'suppressions/[id].delete')
  h.epkExports = (await import('../server/api/v1/epk/exports.get')).default as Handler
})

beforeEach(() => {
  db.resetOutreachMock()
})

async function call(name: string, opts: Partial<FakeEvent> = {}) {
  const e: FakeEvent = { params: {}, status: 200, ...opts }
  const res = (await h[name]!(e)) as never
  return { status: e.status, res }
}

const futureDate = () => {
  const d = new Date(Date.now() + 90 * 86400000)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

const validSlot = () => ({
  date: futureDate(), city: 'Leipzig', country: 'DE', genres: ['techno'],
  fee_min: 400, fee_max: 700, currency: 'EUR', notes: 'Open night.',
})

describe('outreach mocks — availability slots', () => {
  it('lists the seeded slots ordered by date ASC, city ASC', async () => {
    const { status, res } = await call('slotsList')
    expect(status).toBe(200)
    const slots = res as AvailabilitySlot[]
    expect(slots.map(s => s.id)).toEqual(['as-berlin', 'as-hamburg'])
    expect(slots[0]).toMatchObject({ date: '2027-01-16', city: 'Berlin', genres: ['techno', 'house'], fee_min: 600, fee_max: 900, currency: 'EUR' })
  })

  it('creates a slot (201) and rejects a duplicate date/city/country (409 conflict)', async () => {
    const created = await call('slotCreate', { body: validSlot() })
    expect(created.status).toBe(201)
    expect((created.res as AvailabilitySlot).id).toMatch(/^as-/)
    const dup = await call('slotCreate', { body: validSlot() })
    expect(dup.status).toBe(409)
    expect((dup.res as { error: string }).error).toBe('conflict')
  })

  it('validates like the Go service: past date, missing city, fee range, malformed date', async () => {
    const past = await call('slotCreate', { body: { ...validSlot(), date: '2020-01-01' } })
    expect(past.status).toBe(422)
    expect((past.res as { error: string }).error).toBe('validation failed: date: must not be in the past')
    const noCity = await call('slotCreate', { body: { ...validSlot(), city: '  ' } })
    expect((noCity.res as { error: string }).error).toBe('validation failed: city: required')
    const badFee = await call('slotCreate', { body: { ...validSlot(), fee_min: 900, fee_max: 100 } })
    expect((badFee.res as { error: string }).error).toBe('validation failed: fee_min: must not exceed fee_max')
    const badDate = await call('slotCreate', { body: { ...validSlot(), date: 'next friday' } })
    expect(badDate.status).toBe(400)
    expect((badDate.res as { error: string }).error).toBe('invalid JSON body')
    const badCurrency = await call('slotCreate', { body: { ...validSlot(), currency: 'EURO' } })
    expect((badCurrency.res as { error: string }).error).toBe('validation failed: currency: must be a 3-letter ISO 4217 code')
  })

  it('deletes a slot (204) and keeps sent-log history via SET NULL; unknown id is 404', async () => {
    const del = await call('slotDelete', { params: { id: 'as-berlin' } })
    expect(del.status).toBe(204)
    expect((await call('slotsList')).res as AvailabilitySlot[]).toHaveLength(1)
    // or-anna referenced as-berlin: the row survives with a null slot.
    const req = (await call('requestGet', { params: { id: 'or-anna' } })).res as OutreachRequest
    expect(req.availability_slot_id).toBeNull()
    const missing = await call('slotDelete', { params: { id: 'as-nope' } })
    expect(missing.status).toBe(404)
    expect((missing.res as { error: string }).error).toBe('not found')
  })
})

describe('outreach mocks — matches (SPEC §8)', () => {
  it('matches by slot city: suppressed excluded, no-email included, past gig first', async () => {
    const { res } = await call('matches')
    const groups = res as SlotMatch[]
    expect(groups).toHaveLength(2)
    const berlin = groups.find(g => g.slot.city === 'Berlin')!
    // past_gig_in_city DESC, then name ASC: anna, ben, cara, dirk.
    expect(berlin.contacts.map(c => c.id)).toEqual(['pc-anna', 'pc-ben', 'pc-cara', 'pc-dirk'])
    // The no-email contact is matchable but flagged (OUTR-05 v2.1).
    const dirk = berlin.contacts.find(c => c.id === 'pc-dirk')!
    expect(dirk.has_email).toBe(false)
    expect(dirk.email).toBe('')
    // The suppressed contact (frieda) and the Hamburg contact (emil) are out.
    expect(berlin.contacts.some(c => c.id === 'pc-frieda')).toBe(false)
    expect(berlin.contacts.some(c => c.id === 'pc-emil')).toBe(false)
    // Venues in the city, name ASC.
    expect(berlin.venues.map(v => v.name)).toEqual(['Klubnacht', 'Warehouse 7'])
  })

  it('filters by ?slot_id= and 404s on an unknown slot', async () => {
    const { res } = await call('matches', { query: { slot_id: 'as-hamburg' } })
    const groups = res as SlotMatch[]
    expect(groups).toHaveLength(1)
    expect(groups[0]!.slot.city).toBe('Hamburg')
    expect(groups[0]!.contacts.map(c => c.id)).toEqual(['pc-emil'])
    expect(groups[0]!.venues.map(v => v.name)).toEqual(['Elbe Stage'])
    const missing = await call('matches', { query: { slot_id: 'as-nope' } })
    expect(missing.status).toBe(404)
  })
})

describe('outreach mocks — templates', () => {
  it('lists the default first (is_default DESC, name ASC) and seeds migration 034', async () => {
    const { res } = await call('templatesList')
    const templates = res as OutreachTemplate[]
    expect(templates.map(t => t.id)).toEqual(['tpl-default', 'tpl-weekend'])
    expect(templates[0]).toMatchObject({
      name: 'Default booking request', direction: 'dj_to_promoter', is_default: true, version: 1,
      subject: 'Booking request: {{dj_name}} in {{city}} on {{date}}',
    })
    expect(templates[0]!.body).toContain('{{promoter_name}}')
    expect(templates[0]!.body).toContain('{{epk_link}}')
  })

  it('creates (201), rejects duplicate names (409) and empty fields (422)', async () => {
    const created = await call('templateCreate', { body: { name: 'Follow-up', subject: 'Again?', body: 'Ping {{promoter_name}}.', is_default: false } })
    expect(created.status).toBe(201)
    const dup = await call('templateCreate', { body: { name: 'follow-up', subject: 'x', body: 'y', is_default: false } })
    expect(dup.status).toBe(409)
    expect((dup.res as { error: string }).error).toBe('conflict')
    const empty = await call('templateCreate', { body: { name: 'X', subject: '', body: 'y', is_default: false } })
    expect((empty.res as { error: string }).error).toBe('validation failed: subject: required')
  })

  it('updates with the optimistic-concurrency version (409 on mismatch, 422 without it)', async () => {
    const stale = await call('templatePut', { params: { id: 'tpl-weekend' }, body: { version: 99, subject: 'New' } })
    expect(stale.status).toBe(409)
    expect((stale.res as { error: string }).error).toBe('conflict')
    const noVersion = await call('templatePut', { params: { id: 'tpl-weekend' }, body: { subject: 'New' } })
    expect(noVersion.status).toBe(422)
    expect((noVersion.res as { error: string }).error).toBe('validation failed: version: required (optimistic concurrency token)')
    const updated = await call('templatePut', { params: { id: 'tpl-weekend' }, body: { version: 2, subject: 'New subject' } })
    expect(updated.status).toBe(200)
    expect((updated.res as OutreachTemplate).version).toBe(3)
    expect((updated.res as OutreachTemplate).subject).toBe('New subject')
    const missing = await call('templatePut', { params: { id: 'tpl-nope' }, body: { version: 1 } })
    expect(missing.status).toBe(404)
  })

  it('deletes (204) and keeps sent-log history via SET NULL', async () => {
    const del = await call('templateDelete', { params: { id: 'tpl-default' } })
    expect(del.status).toBe(204)
    const req = (await call('requestGet', { params: { id: 'or-anna' } })).res as OutreachRequest
    expect(req.template_id).toBeNull()
    expect((await call('templateDelete', { params: { id: 'tpl-nope' } })).status).toBe(404)
  })
})

describe('outreach mocks — send request', () => {
  it('sends (201): renders the template, stamps delivery, prepends to the log', async () => {
    const { status, res } = await call('requestSend', {
      body: { contact_id: 'pc-cara', availability_slot_id: 'as-berlin', template_id: 'tpl-default', epk_export_id: 'epk-1' },
    })
    expect(status).toBe(201)
    const sent = res as OutreachRequest
    expect(sent.status).toBe('sent')
    expect(sent.email_status).toBe('delivered')
    expect(sent.email_message_id).toMatch(/^plunk-mock-/)
    expect(sent.to_email).toBe('cara@lind.example')
    expect(sent.subject).toBe('Booking request: SAMPLE_RATE in Berlin on 2027-01-16')
    expect(sent.body).toContain('Hi Cara Lind,')
    expect(sent.body).toContain('samplerate@dj.example')
    expect(sent.body).toContain('https://klubhub.example/epk/sample-rate')
    expect(sent.body).toContain('600–900 EUR')
    const list = (await call('requestsList')).res as OutreachRequest[]
    expect(list[0]!.id).toBe(sent.id)
  })

  it('per-request subject/body overrides win over the template', async () => {
    const { res } = await call('requestSend', {
      body: { contact_id: 'pc-cara', subject: 'Custom {{city}}', body: 'Custom body {{dj_name}}' },
    })
    const sent = res as OutreachRequest
    expect(sent.subject).toBe('Custom ')
    expect(sent.body).toBe('Custom body SAMPLE_RATE')
    expect(sent.template_id).toBe('tpl-default')
  })

  it('refuses with the Go service error texts: 422/404/409/503', async () => {
    const noContact = await call('requestSend', { body: {} })
    expect(noContact.status).toBe(422)
    expect((noContact.res as { error: string }).error).toBe('validation failed: contact_id: required')
    const unknown = await call('requestSend', { body: { contact_id: 'pc-nope' } })
    expect(unknown.status).toBe(404)
    expect((unknown.res as { error: string }).error).toBe('not found')
    const noEmail = await call('requestSend', { body: { contact_id: 'pc-dirk' } })
    expect(noEmail.status).toBe(422)
    expect((noEmail.res as { error: string }).error).toBe('contact has no email address')
    const suppressed = await call('requestSend', { body: { contact_id: 'pc-frieda' } })
    expect(suppressed.status).toBe(409)
    expect((suppressed.res as { error: string }).error).toBe('address is suppressed')
    const noTemplate = await call('requestSend', { body: { contact_id: 'pc-cara', template_id: 'tpl-nope' } })
    expect(noTemplate.status).toBe(404)
    const noSlot = await call('requestSend', { body: { contact_id: 'pc-cara', availability_slot_id: 'as-nope' } })
    expect(noSlot.status).toBe(404)

    db.setEmailAvailable(false)
    const noEmailProvider = await call('requestSend', { body: { contact_id: 'pc-cara' } })
    expect(noEmailProvider.status).toBe(503)
    expect((noEmailProvider.res as { error: string }).error).toBe('email is not available')
    db.setEmailAvailable(true)

    db.setDjContactInfo({ email: '' })
    const noReplyTo = await call('requestSend', { body: { contact_id: 'pc-cara' } })
    expect(noReplyTo.status).toBe(422)
    expect((noReplyTo.res as { error: string }).error).toBe('no DJ contact email configured')
  })

  it('refuses a legacy EPK export (422) and an unknown one (404) without logging a row', async () => {
    const before = (await call('requestsList')).res as OutreachRequest[]
    const legacy = await call('requestSend', { body: { contact_id: 'pc-cara', epk_export_id: 'epk-legacy' } })
    expect(legacy.status).toBe(422)
    expect((legacy.res as { error: string }).error).toBe('epk export has no garage object key')
    const unknown = await call('requestSend', { body: { contact_id: 'pc-cara', epk_export_id: 'epk-nope' } })
    expect(unknown.status).toBe(404)
    const after = (await call('requestsList')).res as OutreachRequest[]
    expect(after).toHaveLength(before.length)
  })

  it('enforces the daily cap of 50 (429 daily outreach limit reached)', async () => {
    for (let i = 0; i < 50; i++) {
      const { status } = await call('requestSend', { body: { contact_id: 'pc-cara' } })
      expect(status).toBe(201)
    }
    const capped = await call('requestSend', { body: { contact_id: 'pc-cara' } })
    expect(capped.status).toBe(429)
    expect((capped.res as { error: string }).error).toBe('daily outreach limit reached')
  })
})

describe('outreach mocks — requests + suppressions', () => {
  it('lists the sent log newest first; get 404s on unknown ids', async () => {
    const { res } = await call('requestsList')
    const list = res as OutreachRequest[]
    expect(list.map(r => r.id)).toEqual(['or-anna', 'or-ben'])
    expect(list[0]).toMatchObject({ status: 'sent', email_status: 'delivered', epk_export_id: 'epk-1' })
    expect(list[1]).toMatchObject({ status: 'failed', email_status: 'failed', email_error: 'mailbox full' })
    expect((await call('requestGet', { params: { id: 'or-nope' } })).status).toBe(404)
  })

  it('lists suppressions newest first; add is idempotent; delete 204/404', async () => {
    const { res } = await call('suppressionsList')
    expect((res as Suppression[]).map(s => s.id)).toEqual(['sup-2', 'sup-1'])
    const added = await call('suppressionAdd', { body: { email: 'New@Example.com ', reason: 'asked' } })
    expect(added.status).toBe(201)
    expect((added.res as Suppression).email).toBe('new@example.com')
    const again = await call('suppressionAdd', { body: { email: 'new@example.com', reason: 'other' } })
    expect(again.status).toBe(201)
    expect((again.res as Suppression).id).toBe((added.res as Suppression).id)
    const bad = await call('suppressionAdd', { body: { email: 'not-an-email', reason: '' } })
    expect((bad.res as { error: string }).error).toBe('validation failed: email: must be a valid email address')
    const empty = await call('suppressionAdd', { body: { email: '', reason: '' } })
    expect((empty.res as { error: string }).error).toBe('validation failed: email: required')
    expect((await call('suppressionDelete', { params: { id: 'sup-2' } })).status).toBe(204)
    expect((await call('suppressionDelete', { params: { id: 'sup-nope' } })).status).toBe(404)
  })

  it('a new suppression leaves the matchable pool and refuses sends (409)', async () => {
    await call('suppressionAdd', { body: { email: 'cara@lind.example', reason: 'replied "stop"' } })
    const { res } = await call('matches', { query: { slot_id: 'as-berlin' } })
    const berlin = (res as SlotMatch[])[0]!
    expect(berlin.contacts.some(c => c.id === 'pc-cara')).toBe(false)
    const send = await call('requestSend', { body: { contact_id: 'pc-cara' } })
    expect(send.status).toBe(409)
    expect((send.res as { error: string }).error).toBe('address is suppressed')
  })
})

describe('epk exports mock', () => {
  it('answers {"data": […]} newest first with camelCase rows', async () => {
    const e: FakeEvent = { params: {}, status: 200 }
    const res = (await h.epkExports!(e)) as { data: { id: string, garageObjectKey: string, createdAt: string }[] }
    expect(res.data.map(x => x.id)).toEqual(['epk-1', 'epk-legacy'])
    expect(res.data[0]).toMatchObject({ garageObjectKey: 'epk/exports/epk-1.pdf' })
    expect(res.data[0]!.createdAt).toMatch(/^\d{4}-\d{2}-\d{2}T/)
  })
})
