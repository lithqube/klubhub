// @vitest-environment node
// Demo fetch shim, tracklist parser, PDF writer and social/EPK handlers.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createDemoBackend } from '../app/demo/backend'
import { demoApiPath, installDemoFetch } from '../app/demo/fetch'
import { buildPdf } from '../app/demo/lib/pdf'
import { parseTracklist } from '../app/demo/lib/parser'
import { zonedToUtc } from '../app/demo/lib/util'
import { SAMPLE_TRACKLIST_TXT } from '../app/demo/seed/tracklists'

const make = () => createDemoBackend({ storage: null, baseURL: '/demo/', createObjectUrl: () => 'blob:test/1' })

describe('demo fetch shim', () => {
  const origin = 'http://localhost:4300'
  let uninstall: (() => void) | null = null
  afterEach(() => { uninstall?.(); uninstall = null; vi.unstubAllGlobals() })

  it('recognises API paths under the base URL only', () => {
    const p = (u: string) => demoApiPath(new URL(u, origin), origin, '/demo/')
    expect(p('/demo/api/v1/gigs?x=1')).toBe('/api/v1/gigs')
    expect(p('/api/v1/gigs')).toBe('/api/v1/gigs')
    expect(p('/demo/_nuxt/entry.js')).toBeNull()
    expect(p('https://api.example.com/api/v1/gigs')).toBeNull()
  })

  it('answers API calls in-process and passes everything else through', async () => {
    vi.stubGlobal('location', new URL(`${origin}/demo/gigs`))
    const passthrough = vi.fn(async () => new Response('static'))
    vi.stubGlobal('fetch', passthrough)
    uninstall = installDemoFetch(make(), { baseURL: '/demo/' })

    const res = await fetch('/demo/api/v1/settings')
    expect(res.headers.get('content-type')).toBe('application/json')
    expect((await res.json()).dj_name).toBe('Sam Example')

    const created = await fetch('/demo/api/v1/gigs', {
      method: 'POST', body: JSON.stringify({ date: '2026-12-24T00:00:00Z', venue: 'Club Alpha' }),
    })
    expect(created.status).toBe(201)
    expect((await fetch('/demo/api/v1/gigs/nope', { method: 'DELETE' })).status).toBe(404)
    expect(passthrough).not.toHaveBeenCalled()

    expect(await (await fetch('/demo/_nuxt/app.js')).text()).toBe('static')
    expect(passthrough).toHaveBeenCalledTimes(1)
  })
})

describe('tracklist parser', () => {
  it('reads rekordbox-style TSV with a header row', () => {
    const r = parseTracklist(SAMPLE_TRACKLIST_TXT)
    expect(r.sourceFormat).toBe('rekordbox')
    expect(r.tracks).toHaveLength(12)
    expect(r.tracks[0]).toMatchObject({ bpm: 126, musicalKey: '8A' })
    expect(r.tracks[0]!.durationSecs).toBeGreaterThan(300)
  })

  it('reads rekordbox XML collection tracks and skips playlist refs', () => {
    const xml = '<DJ_PLAYLISTS><COLLECTION><TRACK TrackID="1" Name="Low Orbit" Artist="Artist Alpha" AverageBpm="124.00" TotalTime="361" Tonality="8A"/>'
      + '<TRACK TrackID="2" Name="Soft &amp; Slow" Artist="Demo Unit"/></COLLECTION><PLAYLISTS><TRACK Key="1"/></PLAYLISTS></DJ_PLAYLISTS>'
    const r = parseTracklist(xml)
    expect(r.tracks.map((t) => t.title)).toEqual(['Low Orbit', 'Soft & Slow'])
    expect(r.tracks[0]).toMatchObject({ bpm: 124, durationSecs: 361, musicalKey: '8A' })
  })

  it('reads CSV and plain lines with timestamps', () => {
    expect(parseTracklist('name,artist,bpm\n"Signal, Path",Artist Beta,128').tracks[0]).toMatchObject({ title: 'Signal, Path', artist: 'Artist Beta', bpm: 128 })
    const r = parseTracklist('[0:00] Artist Gamma - Fog Machine\n1:02:03 Sample Collective - Second Room')
    expect(r.tracks.map((t) => t.artist)).toEqual(['Artist Gamma', 'Sample Collective'])
  })
})

describe('demo helpers', () => {
  it('builds a PDF whose xref offsets point at the objects', () => {
    const pdf = buildPdf([{ text: 'Hello (demo)', bold: true, size: 18 }, { text: 'Café — 100 €' }])
    const xref = Number(/startxref\n(\d+)/.exec(pdf)![1])
    expect(pdf.slice(xref, xref + 4)).toBe('xref')
    const first = Number(/\n(\d{10}) 00000 n/.exec(pdf)![1])
    expect(pdf.slice(first, first + 7)).toBe('1 0 obj')
    expect(pdf).toContain('(Hello \\(demo\\)) Tj')
    expect(pdf).toContain('Cafe - 100 EUR')
  })

  it('converts a wall-clock time in a zone to UTC', () => {
    expect(zonedToUtc('2026-07-01T20:00', 'Europe/Berlin')).toBe('2026-07-01T18:00:00.000Z')
    expect(zonedToUtc('2026-01-15T09:30', 'America/New_York')).toBe('2026-01-15T14:30:00.000Z')
  })
})

describe('demo social and EPK', () => {
  const call = async (b: ReturnType<typeof make>, method: string, path: string, body: unknown = null) => {
    const res = await b.handle({ method, path, query: new URLSearchParams(), body })
    return { status: res.status, body: res.body as Record<string, any> }
  }

  it('schedules, retries (simulated publish) and disconnects the fictional account', async () => {
    const b = make()
    const form = new FormData()
    form.append('post_type', 'feed')
    form.append('caption', 'Demo post')
    form.append('scheduled_at', '2099-01-01T20:00')
    form.append('timezone_name', 'UTC')
    form.append('image_file', new File([new Uint8Array([137, 80, 78, 71])], 'a.png', { type: 'image/png' }))
    const created = await call(b, 'POST', '/api/v1/social/posts', form)
    expect(created.status).toBe(201)
    expect(created.body.data).not.toHaveProperty('imageData')
    expect(b.resolveAsset(`/api/v1/social/posts/${created.body.data.id}/image`)).toMatch(/^data:image\/png;base64,/)
    const posts = (await call(b, 'GET', '/api/v1/social/posts')).body.data
    const failed = posts.find((p: { status: string }) => p.status === 'failed')
    await call(b, 'POST', `/api/v1/social/posts/${failed.id}/retry`)
    expect((await call(b, 'GET', '/api/v1/social/posts')).body.data.find((p: { id: string }) => p.id === failed.id).status).toBe('published')
    const account = (await call(b, 'GET', '/api/v1/social/accounts')).body.data
    expect(account.accountName).toBe('sam.example')
    expect((await call(b, 'DELETE', `/api/v1/social/accounts/${account.id}`)).status).toBe(204)
    expect((await call(b, 'GET', '/api/v1/social/auth/url')).body.url).toBe('/demo/social?connected=demo')
  })

  it('stores EPK photos as capped data URLs and exports a PDF', async () => {
    const b = make()
    const big = new FormData()
    big.append('photo', new File([new Uint8Array(900 * 1024)], 'big.jpg', { type: 'image/jpeg' }))
    const tooBig = await call(b, 'POST', '/api/v1/epk/photos', big)
    expect(tooBig.status).toBe(413)
    expect(tooBig.body.error).toMatch(/limited/)
    const ok = new FormData()
    ok.append('photo', new File([new Uint8Array(10)], 'ok.jpg', { type: 'image/jpeg' }))
    const up = await call(b, 'POST', '/api/v1/epk/photos', ok)
    expect(up.body.url).toMatch(/^data:image\/jpeg;base64,/)
    const content = (await call(b, 'GET', '/api/v1/epk/content')).body.data
    expect(content.photoUrls[up.body.path]).toBe(up.body.url)
    const exp = await call(b, 'POST', '/api/v1/epk/export')
    expect(exp.body.downloadUrl).toMatch(/^blob:/)
    expect((await call(b, 'GET', '/api/v1/epk/exports')).body.data).toHaveLength(1)
  })
})
