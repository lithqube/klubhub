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
