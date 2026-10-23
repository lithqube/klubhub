// @vitest-environment node
// Browser demo backend (app/demo, docs/DEMO.md): router, persistence/reset,
// and the tracklist, gig and finance handlers.
import { describe, expect, it } from 'vitest'
import { createDemoBackend } from '../app/demo/backend'
import { DemoRouter, json } from '../app/demo/router'
import { STORAGE_KEY, type KeyValueStorage } from '../app/demo/state'
import { FINANCE_GIG_IDS } from '../shared/finance-mock/seed'
import type { Invoice } from '../app/types/finance'
import type { Gig } from '../app/types/gig'

class MemoryStorage implements KeyValueStorage {
  data = new Map<string, string>()
  getItem(k: string) { return this.data.get(k) ?? null }
  setItem(k: string, v: string) { this.data.set(k, v) }
  removeItem(k: string) { this.data.delete(k) }
}

const NOW = new Date('2026-09-25T12:00:00Z')

function backend(storage: KeyValueStorage | null = new MemoryStorage()) {
  let n = 0
  return createDemoBackend({ storage, baseURL: '/demo/', now: () => NOW, createObjectUrl: () => `blob:test/${++n}` })
}

type B = ReturnType<typeof backend>
async function call(b: B, method: string, path: string, body: unknown = null) {
  const [p = '', qs = ''] = path.split('?')
  const res = await b.handle({ method, path: p, query: new URLSearchParams(qs), body })
  return { status: res.status, body: res.body as Record<string, any>, blob: res.blob }
}

describe('demo router', () => {
  it('matches methods and params; literal routes registered first win', () => {
    const r = new DemoRouter()
      .on('GET', '/api/v1/gigs/autocomplete', () => json('auto'))
      .on('GET', '/api/v1/gigs/:id', () => json('one'))
    expect(r.match('GET', '/api/v1/gigs/autocomplete')?.params).toEqual({})
    expect(r.match('GET', '/api/v1/gigs/a%20b')?.params).toEqual({ id: 'a b' })
    expect(r.match('POST', '/api/v1/gigs/x')).toBeNull()
    expect(r.match('GET', '/api/v1/gigs/x/y')).toBeNull()
  })

  it('unknown API routes answer 404 with the Go error shape', async () => {
    const res = await call(backend(), 'GET', '/api/v1/gigs/info/some-slug/extra')
    expect(res.status).toBe(404)
    expect(typeof res.body.error).toBe('string')
  })
})

describe('demo persistence', () => {
  it('seeds on first visit under a versioned key and survives a reload', async () => {
    const storage = new MemoryStorage()
    const b1 = backend(storage)
    expect(storage.getItem(STORAGE_KEY)).toBeTruthy()
    await call(b1, 'PUT', '/api/v1/epk/content', { bioShort: 'Changed in test' })
    const b2 = backend(storage)
    expect((await call(b2, 'GET', '/api/v1/epk/content')).body.data.bioShort).toBe('Changed in test')
  })

  it('reset drops changes and re-seeds', async () => {
    const storage = new MemoryStorage()
    const b = backend(storage)
    const before = (await call(b, 'GET', '/api/v1/gigs')).body as unknown as Gig[]
    await call(b, 'DELETE', `/api/v1/gigs/${before[0]!.id}`)
    expect(((await call(b, 'GET', '/api/v1/gigs')).body as unknown as Gig[]).length).toBe(before.length - 1)
    b.reset()
    expect(((await call(b, 'GET', '/api/v1/gigs')).body as unknown as Gig[]).length).toBe(before.length)
    const inv = (await call(b, 'GET', '/api/v1/finance/invoices')).body.data as Invoice[]
    expect(inv.length).toBeGreaterThan(0)
  })

  it('falls back to memory when storage is missing, throws or holds garbage', async () => {
    const broken: KeyValueStorage = {
      getItem: () => { throw new Error('blocked') },
      setItem: () => { throw new Error('quota') },
      removeItem: () => { throw new Error('blocked') },
    }
    for (const s of [null, broken]) {
      const b = backend(s)
      expect(b.persistent).toBe(false)
      expect((await call(b, 'POST', '/api/v1/gigs', { date: '2026-12-01T00:00:00Z', venue: 'Club Alpha' })).status).toBe(201)
      b.reset()
    }
    const garbage = new MemoryStorage()
    garbage.setItem(STORAGE_KEY, '{not json')
    expect((await call(backend(garbage), 'GET', '/api/v1/settings')).body.dj_name).toBe('Sam Example')
  })

  it('seed data is fictional: every email uses an example domain', () => {
    const text = JSON.stringify(backend().state)
    const emails = text.match(/[\w.+-]+@[\w.-]+\.[a-z]+/gi) ?? []
    expect(emails.length).toBeGreaterThan(5)
    for (const e of emails) expect(e.split('@')[1]).toMatch(/(^|\.)example(\.(com|test))?$/)
  })
})

describe('demo tracklists', () => {
  it('parses an uploaded export and lists it first', async () => {
    const b = backend()
    const form = new FormData()
    form.append('file', new File(['01. Artist Alpha - Low Orbit [124]\n02. Demo Unit - Soft Reset\nno separator here'], 'my set.txt'))
    const res = await call(b, 'POST', '/api/v1/tracklists/upload', form)
    expect(res.status).toBe(201)
    expect(res.body.tracklist.title).toBe('MY SET')
    expect(res.body.tracks.map((t: { artist: string }) => t.artist)).toEqual(['Artist Alpha', 'Demo Unit', ''])
    expect(res.body.tracks[0].bpm).toBe(124)
    expect(res.body.warnings).toHaveLength(1)
    const list = (await call(b, 'GET', '/api/v1/tracklists')).body.data
    expect(list[0].id).toBe(res.body.tracklist.id)
  })

  it('rejects files without tracks (422) and generates export images', async () => {
    const b = backend()
    const form = new FormData()
    form.append('file', new File(['\n\n'], 'empty.txt'))
    expect((await call(b, 'POST', '/api/v1/tracklists/upload', form)).status).toBe(422)
    const [tl] = (await call(b, 'GET', '/api/v1/tracklists')).body.data
    const img = await call(b, 'POST', `/api/v1/tracklists/${tl.id}/generate-image?format=both`)
    expect(Object.keys(img.body).sort()).toEqual(['square', 'story'])
    expect((await call(b, 'POST', `/api/v1/tracklists/${tl.id}/generate-image?format=poster`)).status).toBe(400)
  })

  it('renames a tracklist like the Go API (trimmed, 1-200 chars) and lists its linked gigs', async () => {
    const b = backend()
    const [tl] = (await call(b, 'GET', '/api/v1/tracklists')).body.data

    const ok = await call(b, 'PUT', `/api/v1/tracklists/${tl.id}`, { title: '  Friday at Tresor ' })
    expect(ok.status).toBe(200)
    expect(ok.body.title).toBe('Friday at Tresor')
    expect((await call(b, 'GET', `/api/v1/tracklists/${tl.id}`)).body.tracklist.title).toBe('Friday at Tresor')
    expect((await call(b, 'GET', '/api/v1/tracklists')).body.data.find((t: { id: string }) => t.id === tl.id).title).toBe('Friday at Tresor')

    for (const body of [{}, { title: '   ' }, { title: 'x'.repeat(201) }]) {
      expect((await call(b, 'PUT', `/api/v1/tracklists/${tl.id}`, body)).status).toBe(422)
    }
    expect((await call(b, 'PUT', '/api/v1/tracklists/nope', { title: 'x' })).status).toBe(404)

    // A seeded gig has a linked tracklist: it shows up, and unlinking removes it.
    const detail = (await call(b, 'GET', `/api/v1/gigs/${FINANCE_GIG_IDS.domestic}/detail`)).body
    const linkedId = detail.tracklists[0].id
    const gigs = (await call(b, 'GET', `/api/v1/tracklists/${linkedId}/gigs`)).body as unknown as Array<{ id: string; venue: string; eventName: string }>
    expect(gigs.map((g) => g.id)).toContain(FINANCE_GIG_IDS.domestic)
    expect(gigs[0]).toHaveProperty('eventName')
    await call(b, 'DELETE', `/api/v1/gigs/${FINANCE_GIG_IDS.domestic}/tracklists/${linkedId}`)
    const after = (await call(b, 'GET', `/api/v1/tracklists/${linkedId}/gigs`)).body as unknown as Array<{ id: string }>
    expect(after.map((g) => g.id)).not.toContain(FINANCE_GIG_IDS.domestic)
    expect((await call(b, 'GET', '/api/v1/tracklists/nope/gigs')).status).toBe(404)
  })

  it('adds a track by hand, reorders tracks and keeps the hidden-gem / unreleased / media markers', async () => {
    const b = backend()
    const [tl] = (await call(b, 'GET', '/api/v1/tracklists')).body.data
    // A copy: the demo returns its live state array, which the POST below grows.
    const before = [...((await call(b, 'GET', `/api/v1/tracklists/${tl.id}`)).body.tracks as Array<{ id: string; position: number }>)]

    const added = await call(b, 'POST', `/api/v1/tracklists/${tl.id}/tracks`, { title: ' New ID ', artist: 'Me', bpm: 128, media: 'vinyl', hiddenGem: true, unreleased: true })
    expect(added.status).toBe(201)
    expect(added.body).toMatchObject({ title: 'New ID', position: before.length + 1, media: 'vinyl', hiddenGem: true, unreleased: true })
    for (const bad of [{ title: ' ' }, { title: 'x', media: 'wav' }, { title: 'x', bpm: 1000 }, { title: 'x'.repeat(201) }]) {
      expect((await call(b, 'POST', `/api/v1/tracklists/${tl.id}/tracks`, bad)).status).toBe(422)
    }
    expect((await call(b, 'POST', '/api/v1/tracklists/nope/tracks', { title: 'x' })).status).toBe(404)

    // Reverse the order: every id once, no more, no fewer.
    const ids = [...before.map((t) => t.id), added.body.id as string]
    const reversed = ids.slice().reverse()
    const ok = await call(b, 'PUT', `/api/v1/tracklists/${tl.id}/tracks/order`, { trackIds: reversed })
    expect(ok.status).toBe(200)
    expect((ok.body as unknown as Array<{ id: string; position: number }>).map((t) => [t.id, t.position])).toEqual(reversed.map((id, i) => [id, i + 1]))
    expect((await call(b, 'GET', `/api/v1/tracklists/${tl.id}`)).body.tracks.map((t: { id: string }) => t.id)).toEqual(reversed)
    for (const bad of [reversed.slice(1), [...reversed.slice(1), reversed[1]], [], ['nope']]) {
      expect((await call(b, 'PUT', `/api/v1/tracklists/${tl.id}/tracks/order`, { trackIds: bad })).status).toBe(422)
    }

    // Markers update one at a time; only vinyl and digital are accepted.
    const first = reversed[0]!
    const put = (body: unknown) => call(b, 'PUT', `/api/v1/tracklists/${tl.id}/tracks/${first}`, body)
    expect((await put({ media: 'digital' })).body.media).toBe('digital')
    expect((await put({ hiddenGem: false })).body).toMatchObject({ hiddenGem: false, unreleased: true, media: 'digital' })
    expect((await put({ media: '' })).body.media).toBe('')
    expect((await put({ media: 'mp3' })).status).toBe(422)
  })

  it('edits a track and deletes a tracklist (204)', async () => {
    const b = backend()
    const [tl] = (await call(b, 'GET', '/api/v1/tracklists')).body.data
    const { tracks } = (await call(b, 'GET', `/api/v1/tracklists/${tl.id}`)).body
    const put = await call(b, 'PUT', `/api/v1/tracklists/${tl.id}/tracks/${tracks[0].id}`, { title: 'Renamed', bpm: 130 })
    expect(put.body).toMatchObject({ title: 'Renamed', bpm: 130 })
    expect((await call(b, 'DELETE', `/api/v1/tracklists/${tl.id}`)).status).toBe(204)
    expect((await call(b, 'GET', `/api/v1/tracklists/${tl.id}`)).status).toBe(404)
  })
})

describe('demo gigs', () => {
  it('lists newest first, filters by status and covers every status', async () => {
    const b = backend()
    const gigs = (await call(b, 'GET', '/api/v1/gigs')).body as unknown as Gig[]
    expect(gigs.length).toBeGreaterThanOrEqual(8)
    expect([...gigs].sort((a, b2) => b2.date.localeCompare(a.date))).toEqual(gigs)
    expect(new Set(gigs.map((g) => g.status))).toEqual(new Set(['played', 'confirmed', 'advanced', 'inquiry', 'cancelled']))
    const played = (await call(b, 'GET', '/api/v1/gigs?status=played')).body as unknown as Gig[]
    expect(played.every((g) => g.status === 'played')).toBe(true)
  })

  it('creates, updates with a concurrency token and serves details', async () => {
    const b = backend()
    const created = (await call(b, 'POST', '/api/v1/gigs', {
      date: '2026-11-02T00:00:00Z', venue: 'Gamma Hall', event_name: 'Test Night', fee_amount: 500, fee_currency: 'eur', status: 'confirmed',
    }))
    expect(created.status).toBe(201)
    const gig = created.body as unknown as Gig
    expect(gig.fee_currency).toBe('EUR')
    expect((await call(b, 'PUT', `/api/v1/gigs/${gig.id}`, { status: 'played', updated_at: 'stale' })).status).toBe(409)
    const ok = await call(b, 'PUT', `/api/v1/gigs/${gig.id}`, { status: 'played', updated_at: gig.updated_at })
    expect(ok.body.status).toBe('played')
    const detail = (await call(b, 'GET', `/api/v1/gigs/${FINANCE_GIG_IDS.domestic}/detail`)).body
    expect(detail.tracklists).toHaveLength(1)
    expect(detail.linked_venues[0].venue.name).toBe('Club Alpha')
  })

  it('exports an iCal feed and a booking PDF', async () => {
    const b = backend()
    const ics = await (await call(b, 'GET', '/api/v1/gigs/calendar.ics')).blob!.text()
    expect(ics).toMatch(/^BEGIN:VCALENDAR/)
    expect(ics.match(/BEGIN:VEVENT/g)?.length).toBeGreaterThanOrEqual(8)
    const pdf = await (await call(b, 'GET', `/api/v1/gigs/${FINANCE_GIG_IDS.domestic}/pdf`)).blob!.text()
    expect(pdf.startsWith('%PDF-1.4')).toBe(true)
    expect(pdf).toContain('Booking confirmation')
  })
})

describe('demo finance (shared finance-mock core)', () => {
  it('seeds invoices in every state, including reverse charge, US withholding and a credit note', async () => {
    const list = (await call(backend(), 'GET', '/api/v1/finance/invoices')).body.data as Invoice[]
    expect(new Set(list.map((i) => i.status))).toEqual(new Set(['paid', 'issued', 'draft', 'credited']))
    expect(list.some((i) => i.vat_treatment === 'reverse_charge' && i.status === 'issued')).toBe(true)
    expect(list.some((i) => i.currency === 'USD' && i.withholding_rate_bps === 3000)).toBe(true)
    expect(list.some((i) => i.kind === 'credit_note' && i.invoice_number?.startsWith('CN-'))).toBe(true)
  })

  it('creates a draft from a played gig using its fee and customer, then issues it', async () => {
    const b = backend()
    const res = await call(b, 'POST', '/api/v1/finance/invoices', { gig_id: FINANCE_GIG_IDS.unbilled })
    expect(res.status).toBe(201)
    const draft = res.body.data as Invoice
    expect(draft).toMatchObject({ status: 'draft', invoice_number: null, currency: 'EUR', subtotal_minor: 60000, vat_treatment: 'domestic' })
    expect(draft.customer.legal_name).toBe('Zeta Collective')
    const again = await call(b, 'POST', '/api/v1/finance/invoices', { gig_id: FINANCE_GIG_IDS.unbilled })
    expect(again.status).toBe(409)
    expect(again.body.error).toBe('bad_state')
    const issued = await call(b, 'POST', `/api/v1/finance/invoices/${draft.id}/issue`, { updated_at: draft.updated_at })
    expect(issued.status).toBe(200)
    expect((issued.body.data as Invoice).invoice_number).toMatch(/^INV-\d{4}-EUR$/)
  })

  it('bills a gig created in the demo from its own fee', async () => {
    const b = backend()
    const gig = (await call(b, 'POST', '/api/v1/gigs', {
      date: '2026-09-01T00:00:00Z', venue: 'Theta Bar', country: 'AT', fee_amount: 450.5, fee_currency: 'EUR', status: 'played', promoter_name: 'Theta Bar OG',
    })).body as unknown as Gig
    const draft = (await call(b, 'POST', '/api/v1/finance/invoices', { gig_id: gig.id })).body.data as Invoice
    expect(draft.subtotal_minor).toBe(45050)
    expect(draft.customer).toMatchObject({ legal_name: 'Theta Bar OG', country: 'AT' })
    const check = (await call(b, 'GET', `/api/v1/finance/invoices/${draft.id}/issue-check`)).body.data
    expect(check.ready).toBe(false)
    const issue = await call(b, 'POST', `/api/v1/finance/invoices/${draft.id}/issue`, { updated_at: draft.updated_at })
    expect(issue.status).toBe(422)
    expect(issue.body.error).toBe('not_issuable')
  })

  it('finance state persists across reloads', async () => {
    const storage = new MemoryStorage()
    const b1 = backend(storage)
    const draft = (await call(b1, 'POST', '/api/v1/finance/invoices', { gig_id: FINANCE_GIG_IDS.unbilled })).body.data as Invoice
    const got = await call(backend(storage), 'GET', `/api/v1/finance/invoices/${draft.id}`)
    expect(got.status).toBe(200)
    expect(got.body.lines).toHaveLength(1)
  })
})

describe('demo rider (shared rider-mock core)', () => {
  const GIG = FINANCE_GIG_IDS.unbilled // any gig that exists in the seed

  it('seeds the standard templates and serves them like the Go API', async () => {
    const b = backend()
    const list = await call(b, 'GET', '/api/v1/rider/templates')
    expect(list.status).toBe(200)
    expect((list.body.data as { name: string }[]).map((t) => t.name)).toEqual(['Boilerplate', 'Festival heavy', 'Standard club'])
    const one = await call(b, 'GET', '/api/v1/rider/templates/tmpl-1')
    expect(one.body.data).toMatchObject({ id: 'tmpl-1', name: 'Standard club' })
    expect((await call(b, 'GET', '/api/v1/rider/templates/nope')).status).toBe(404)
  })

  it('rejects what the API rejects: stale/missing token, duplicate name, over-long text', async () => {
    const b = backend()
    const created = (await call(b, 'POST', '/api/v1/rider/templates', { name: 'Mine', technical: 't' }))
    expect(created.status).toBe(201)
    const t = created.body.data

    expect((await call(b, 'POST', '/api/v1/rider/templates', { name: 'mine' })).body).toMatchObject({ error: 'validation_failed' })
    expect((await call(b, 'PUT', `/api/v1/rider/templates/${t.id}`, { technical: 'x' })).status).toBe(422)
    expect((await call(b, 'PUT', `/api/v1/rider/templates/${t.id}`, { technical: 'x', updatedAt: '1999-01-01T00:00:00.000Z' })).body).toMatchObject({ error: 'conflict' })
    expect((await call(b, 'PUT', `/api/v1/rider/templates/${t.id}`, { technical: 'x'.repeat(20001), updatedAt: t.updatedAt })).status).toBe(422)

    const ok = await call(b, 'PUT', `/api/v1/rider/templates/${t.id}`, { technical: 'new', updatedAt: t.updatedAt })
    expect(ok.status).toBe(200)
    expect(ok.body.data).toMatchObject({ technical: 'new', name: 'Mine' })
    expect(ok.body.data.updatedAt > t.updatedAt).toBe(true)
  })

  it('attaches to a gig that exists, once; unknown gig is 404; by-gig is null before and after', async () => {
    const b = backend()
    expect((await call(b, 'GET', `/api/v1/rider/attachments/by-gig/${GIG}`)).body).toEqual({ data: null })

    const unknown = await call(b, 'POST', '/api/v1/rider/attachments', { gigId: 'no-such-gig' })
    expect(unknown.status).toBe(404)

    const att = await call(b, 'POST', '/api/v1/rider/attachments', { gigId: GIG, templateId: 'tmpl-1' })
    expect(att.status).toBe(201)
    expect(att.body.data.technical).toContain('CDJ-3000') // a snapshot of the template
    expect((await call(b, 'POST', '/api/v1/rider/attachments', { gigId: GIG })).status).toBe(409)
    expect((await call(b, 'GET', `/api/v1/rider/attachments/by-gig/${GIG}`)).body.data.id).toBe(att.body.data.id)

    expect((await call(b, 'DELETE', `/api/v1/rider/attachments/${att.body.data.id}`)).status).toBe(204)
    expect((await call(b, 'GET', `/api/v1/rider/attachments/by-gig/${GIG}`)).body).toEqual({ data: null })
  })

  it('deleting a template detaches it from attachments but keeps their text', async () => {
    const b = backend()
    const att = (await call(b, 'POST', '/api/v1/rider/attachments', { gigId: GIG, templateId: 'tmpl-2' })).body.data
    expect((await call(b, 'DELETE', '/api/v1/rider/templates/tmpl-2')).status).toBe(204)
    const got = (await call(b, 'GET', `/api/v1/rider/attachments/${att.id}`)).body.data
    expect(got.templateId).toBeNull()
    expect(got.technical).toBe(att.technical)
    expect(got.updatedAt).toBe(att.updatedAt)
  })

  it('exports a PDF as a bare {id, downloadUrl, createdAt} envelope with a blob URL', async () => {
    const b = backend()
    const att = (await call(b, 'POST', '/api/v1/rider/attachments', { gigId: GIG, templateId: 'tmpl-1' })).body.data
    const res = await call(b, 'POST', `/api/v1/rider/attachments/${att.id}/pdf`)
    expect(res.status).toBe(201)
    expect(res.body).toMatchObject({ id: att.id, downloadUrl: expect.stringMatching(/^blob:/) })
    expect(res.body.data).toBeUndefined()
    expect((await call(b, 'POST', '/api/v1/rider/attachments/nope/pdf')).status).toBe(404)
  })

  it('rider state persists across reloads and reset restores the seed', async () => {
    const storage = new MemoryStorage()
    const b1 = backend(storage)
    await call(b1, 'POST', '/api/v1/rider/templates', { name: 'Persisted' })
    const b2 = backend(storage)
    expect(((await call(b2, 'GET', '/api/v1/rider/templates')).body.data as { name: string }[]).map((t) => t.name)).toContain('Persisted')

    b2.reset()
    expect(((await call(b2, 'GET', '/api/v1/rider/templates')).body.data as unknown[]).length).toBe(3)
  })

  it('a demo state saved before rider existed loads and gets the seed (no crash)', async () => {
    const storage = new MemoryStorage()
    backend(storage) // writes a fresh state
    const stored = JSON.parse(storage.data.get(STORAGE_KEY)!)
    delete stored.rider
    storage.data.set(STORAGE_KEY, JSON.stringify(stored))

    const b = backend(storage)
    const list = await call(b, 'GET', '/api/v1/rider/templates')
    expect(list.status).toBe(200)
    expect((list.body.data as unknown[]).length).toBe(3)
  })
})

describe('demo outreach', () => {
  it('seeds slots, matches, templates and a sent request', async () => {
    const b = backend()
    const slots = (await call(b, 'GET', '/api/v1/outreach/availability/slots')).body
    expect(slots.length).toBe(2)
    expect(slots[0].city).toBe('Berlin')
    expect(slots[1].city).toBe('Paris')

    const matches = (await call(b, 'GET', '/api/v1/outreach/availability/matches')).body
    expect(matches.length).toBe(2)
    const berlin = matches.find((m: any) => m.slot.city === 'Berlin')
    expect(berlin.contacts.length).toBeGreaterThan(0)
    expect(berlin.venues.length).toBeGreaterThan(0)
    // past_gig_in_city ranks first
    expect(berlin.contacts[0].past_gig_in_city).toBe(true)

    const templates = (await call(b, 'GET', '/api/v1/outreach/templates')).body
    expect(templates.length).toBe(1)
    expect(templates[0].is_default).toBe(true)

    const requests = (await call(b, 'GET', '/api/v1/outreach/requests')).body
    expect(requests.length).toBe(1)
    expect(requests[0].status).toBe('sent')
    expect(requests[0].email_status).toBe('delivered')
  })

  it('creates a slot, rejects past date and duplicate city/date/country', async () => {
    const b = backend()
    const future = new Date()
    future.setUTCDate(future.getUTCDate() + 10)
    const date = future.toISOString().slice(0, 10)
    const created = await call(b, 'POST', '/api/v1/outreach/availability/slots', { date, city: 'Amsterdam', country: 'NL', genres: ['house'], fee_min: 400, fee_max: 600, currency: 'EUR', notes: '' })
    expect(created.status).toBe(201)

    const past = await call(b, 'POST', '/api/v1/outreach/availability/slots', { date: '2020-01-01', city: 'Berlin', country: 'DE', genres: [], fee_min: null, fee_max: null, currency: 'EUR', notes: '' })
    expect(past.status).toBe(422)
    expect(past.body.error).toContain('must not be in the past')

    const dup = await call(b, 'POST', '/api/v1/outreach/availability/slots', { date, city: 'Amsterdam', country: 'NL', genres: ['house'], fee_min: 400, fee_max: 600, currency: 'EUR', notes: '' })
    expect(dup.status).toBe(409)
    expect(dup.body.error).toBe('conflict')
  })

  it('lists templates, creates, updates with version token, and deletes', async () => {
    const b = backend()
    const created = await call(b, 'POST', '/api/v1/outreach/templates', { name: 'Custom', subject: 'Sub', body: 'Body', is_default: false })
    expect(created.status).toBe(201)
    expect(created.body.version).toBe(1)
    const id = created.body.id
    const dup = await call(b, 'POST', '/api/v1/outreach/templates', { name: 'Custom', subject: 'Sub', body: 'Body', is_default: false })
    expect(dup.status).toBe(409)

    const updated = await call(b, 'PUT', `/api/v1/outreach/templates/${id}`, { subject: 'New Sub', version: 1 })
    expect(updated.status).toBe(200)
    expect(updated.body.subject).toBe('New Sub')
    expect(updated.body.version).toBe(2)

    const stale = await call(b, 'PUT', `/api/v1/outreach/templates/${id}`, { subject: 'X', version: 1 })
    expect(stale.status).toBe(409)
    expect(stale.body.error).toBe('conflict')

    const del = await call(b, 'DELETE', `/api/v1/outreach/templates/${id}`)
    expect(del.status).toBe(204)
    expect((await call(b, 'GET', `/api/v1/outreach/templates/${id}`)).status).toBe(404)
  })

  it('sends a request, records it with rendered copy and EPK link, enforces daily cap', async () => {
    const b = backend()
    const contact = (await call(b, 'GET', '/api/v1/contacts')).body.find((c: any) => c.email === 'alex@alpha.example')
    const slot = (await call(b, 'GET', '/api/v1/outreach/availability/slots')).body[0]
    const tpl = (await call(b, 'GET', '/api/v1/outreach/templates')).body[0]

    const sent = await call(b, 'POST', '/api/v1/outreach/requests', { contact_id: contact.id, availability_slot_id: slot.id, template_id: tpl.id })
    expect(sent.status).toBe(201)
    expect(sent.body.contact_id).toBe(contact.id)
    expect(sent.body.subject).toContain('Berlin')
    expect(sent.body.status).toBe('sent')

    const requests = (await call(b, 'GET', '/api/v1/outreach/requests')).body
    expect(requests[0].id).toBe(sent.body.id)
  })

  it('rejects a contact without email, a suppressed email', async () => {
    const b = backend()
    const contact = (await call(b, 'GET', '/api/v1/contacts')).body.find((c: any) => c.email === 'alex@alpha.example')
    const slot = (await call(b, 'GET', '/api/v1/outreach/availability/slots')).body[0]
    const tpl = (await call(b, 'GET', '/api/v1/outreach/templates')).body[0]
    // create a contact without email
    const noEmail = await call(b, 'POST', '/api/v1/contacts', { name: 'No Email', company: 'Test', email: '', phone: '0', type: 'promoter' })

    const r1 = await call(b, 'POST', '/api/v1/outreach/requests', { contact_id: noEmail.body.id, availability_slot_id: slot.id, template_id: tpl.id })
    expect(r1.status).toBe(422)
    expect(r1.body.error).toContain('no email address')

    // suppressed email
    const suppressed = await call(b, 'POST', '/api/v1/outreach/suppressions', { email: 'alex@alpha.example', reason: 'test' })
    expect(suppressed.status).toBe(201)
    const r2 = await call(b, 'POST', '/api/v1/outreach/requests', { contact_id: contact.id, availability_slot_id: slot.id, template_id: tpl.id })
    expect(r2.status).toBe(409)
    expect(r2.body.error).toContain('suppressed')
  })

  it('suppressions list, add (idempotent), and delete', async () => {
    const b = backend()
    const list = (await call(b, 'GET', '/api/v1/outreach/suppressions')).body
    expect(list.length).toBe(2)

    const added = await call(b, 'POST', '/api/v1/outreach/suppressions', { email: 'new@example.com', reason: 'manual' })
    expect(added.status).toBe(201)
    const again = await call(b, 'POST', '/api/v1/outreach/suppressions', { email: 'new@example.com', reason: 'again' })
    expect(again.status).toBe(201)
    expect(again.body.id).toBe(added.body.id)

    await call(b, 'DELETE', `/api/v1/outreach/suppressions/${added.body.id}`)
    expect((await call(b, 'GET', `/api/v1/outreach/suppressions/${added.body.id}`)).status).toBe(404)
  })

  it('slot match by slot_id returns single group, unknown slot is 404', async () => {
    const b = backend()
    const slots = (await call(b, 'GET', '/api/v1/outreach/availability/slots')).body
    const match = await call(b, 'GET', `/api/v1/outreach/availability/matches?slot_id=${slots[0].id}`)
    expect(match.status).toBe(200)
    expect(match.body.length).toBe(1)
    expect(match.body[0].slot.id).toBe(slots[0].id)

    expect((await call(b, 'GET', '/api/v1/outreach/availability/matches?slot_id=00000000-0000-4000-8000-000000000999')).status).toBe(404)
  })
})
