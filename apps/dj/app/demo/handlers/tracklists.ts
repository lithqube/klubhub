// /api/v1/tracklists/* — upload/parse, list, edit, artwork and image export.
// Artwork "lookups" are simulated with generated covers; no external calls.

import { uuid } from '../../../shared/finance-mock/rules'
import type { Track } from '../../types/tracklist'
import { coverDataUrl, trackcardSvg } from '../lib/art'
import { parseTracklist } from '../lib/parser'
import { imageDataUrl, readText, TooLargeError } from '../lib/util'
import type { DemoRouter } from '../router'
import { apiError, json, noContent, notFound } from '../router'
import { newTracklist } from '../seed/tracklists'
import type { DemoContext } from '../types'

const MAX_FILE_BYTES = 2 * 1024 * 1024
const EDITABLE = ['title', 'artist', 'album', 'genre', 'bpm', 'rating', 'musicalKey'] as const
const MEDIA = ['', 'vinyl', 'digital']
const MAX_TEXT = 200

function find(c: DemoContext, id: string) {
  return c.state.tracklists.find((t) => t.id === id)
}

function titleFrom(filename: string): string {
  const base = filename.replace(/\.[^.]+$/, '').replace(/[_-]+/g, ' ').trim()
  return (base || 'Imported tracklist').toUpperCase()
}

export function registerTracklists(r: DemoRouter): void {
  r.on('GET', '/api/v1/tracklists', (_req, c) => json({ data: c.state.tracklists }))

  r.on('POST', '/api/v1/tracklists/upload', async (req, c) => {
    const file = req.body instanceof FormData ? req.body.get('file') : null
    if (!(file instanceof Blob)) return apiError(400, 'missing file')
    if (file.size > MAX_FILE_BYTES) return apiError(413, 'file too large (2 MB limit in the demo)')
    const parsed = parseTracklist(await readText(file))
    if (parsed.tracks.length === 0) return apiError(422, 'no tracks found in file')
    const now = c.now().toISOString()
    const name = typeof (file as File).name === 'string' ? (file as File).name : 'tracklist.txt'
    const tl = newTracklist(uuid(), titleFrom(name), parsed.sourceFormat, now, parsed.tracks.length)
    tl.rawFilePath = `demo/uploads/${tl.id}/${name}`
    const tracks: Track[] = parsed.tracks.map((t, i) => ({
      ...t,
      id: uuid(),
      tracklistId: tl.id,
      position: i + 1,
      dateAdded: now,
      artworkStatus: 'fetched',
      artworkUrl: coverDataUrl(`${t.artist}|${t.title}`),
      artworkSource: 'demo',
    }))
    c.state.tracklists.unshift(tl)
    c.state.tracks[tl.id] = tracks
    return json({ tracklist: tl, tracks, warnings: parsed.warnings }, 201)
  })

  r.on('GET', '/api/v1/tracklists/:id', (req, c) => {
    const tl = find(c, req.params.id!)
    if (!tl) return notFound('tracklist not found')
    // `data` is an extra alias for callers that expect the { data } envelope.
    return json({ tracklist: tl, tracks: c.state.tracks[tl.id] ?? [], data: tl })
  })

  // Rename. Mirrors the Go API: trimmed, 1-200 characters, no NUL.
  r.on('PUT', '/api/v1/tracklists/:id', (req, c) => {
    const tl = find(c, req.params.id!)
    if (!tl) return notFound('tracklist not found')
    const raw = (req.body as { title?: unknown } | null)?.title
    if (raw === undefined || raw === null) return apiError(422, 'title is required')
    const title = String(raw).trim()
    if (!title || [...title].length > 200 || title.includes('\0')) return apiError(422, 'title must be 1-200 characters')
    tl.title = title
    tl.updatedAt = c.now().toISOString()
    return json(tl)
  })

  // The gigs a tracklist is linked to (links are managed by the gig routes).
  r.on('GET', '/api/v1/tracklists/:id/gigs', (req, c) => {
    if (!find(c, req.params.id!)) return notFound('tracklist not found')
    const linked = c.state.gigs
      .filter((g) => !g.deleted_at && c.state.gigLinks[g.id]?.tracklists.includes(req.params.id!))
      .sort((a, b) => b.date.localeCompare(a.date))
      .map((g) => ({ id: g.id, date: g.date, venue: g.venue, city: g.city, eventName: g.event_name }))
    return json(linked)
  })

  r.on('DELETE', '/api/v1/tracklists/:id', (req, c) => {
    const i = c.state.tracklists.findIndex((t) => t.id === req.params.id)
    if (i === -1) return notFound('tracklist not found')
    c.state.tracklists.splice(i, 1)
    Reflect.deleteProperty(c.state.tracks, req.params.id!)
    for (const links of Object.values(c.state.gigLinks)) links.tracklists = links.tracklists.filter((t) => t !== req.params.id)
    return noContent()
  })

  // Add a track by hand: appended, validated like the Go API.
  r.on('POST', '/api/v1/tracklists/:id/tracks', (req, c) => {
    const tl = find(c, req.params.id!)
    if (!tl) return notFound('tracklist not found')
    const body = (req.body ?? {}) as Record<string, unknown>
    const title = String(body.title ?? '').trim()
    const artist = String(body.artist ?? '').trim()
    const musicalKey = String(body.musicalKey ?? '').trim()
    const media = String(body.media ?? '')
    const bpm = body.bpm === undefined || body.bpm === null ? 0 : Number(body.bpm)
    if (!title || [...title].length > MAX_TEXT || [...artist].length > MAX_TEXT || [...musicalKey].length > 10
      || !MEDIA.includes(media) || !Number.isFinite(bpm) || bpm < 0 || bpm > 999) {
      return apiError(422, 'a track needs a title (up to 200 characters) and a valid media type')
    }
    const list = (c.state.tracks[tl.id] ??= [])
    const track = {
      id: uuid(), tracklistId: tl.id, position: Math.max(list.length, ...list.map((t) => t.position)) + 1,
      title, artist, album: '', genre: '', bpm, rating: 0, durationSecs: 0, musicalKey,
      dateAdded: c.now().toISOString().slice(0, 10), artworkStatus: 'placeholder' as const, artworkUrl: '', artworkSource: '',
      hiddenGem: body.hiddenGem === true, unreleased: body.unreleased === true, media: media as Track['media'],
    }
    list.push(track)
    return json(track, 201)
  })

  // Reorder: the body lists every track id once, in the new order.
  r.on('PUT', '/api/v1/tracklists/:id/tracks/order', (req, c) => {
    const tl = find(c, req.params.id!)
    if (!tl) return notFound('tracklist not found')
    const list = c.state.tracks[tl.id] ?? []
    const ids = ((req.body as { trackIds?: unknown } | null)?.trackIds ?? []) as unknown
    if (!Array.isArray(ids) || ids.length !== list.length || new Set(ids).size !== ids.length
      || !ids.every((id) => list.some((t) => t.id === id))) {
      return apiError(422, 'trackIds must list every track of the tracklist exactly once')
    }
    const ordered = (ids as string[]).map((id, i) => {
      const t = list.find((x) => x.id === id)!
      t.position = i + 1
      return t
    })
    c.state.tracks[tl.id] = ordered
    return json(ordered)
  })

  r.on('PUT', '/api/v1/tracklists/:id/tracks/:trackId', (req, c) => {
    const track = c.state.tracks[req.params.id!]?.find((t) => t.id === req.params.trackId)
    if (!track) return notFound('track not found')
    const body = (req.body ?? {}) as Record<string, unknown>
    if (body.media !== undefined && body.media !== null && !MEDIA.includes(String(body.media))) {
      return apiError(422, 'invalid track fields')
    }
    if (typeof body.hiddenGem === 'boolean') track.hiddenGem = body.hiddenGem
    if (typeof body.unreleased === 'boolean') track.unreleased = body.unreleased
    if (typeof body.media === 'string') track.media = body.media as Track['media']
    for (const k of EDITABLE) {
      if (body[k] === undefined || body[k] === null) continue
      ;(track as unknown as Record<string, unknown>)[k] = k === 'bpm' || k === 'rating' ? Number(body[k]) || 0 : String(body[k])
    }
    return json(track)
  })

  r.on('PUT', '/api/v1/tracklists/:id/tracks/:trackId/artwork', async (req, c) => {
    const track = c.state.tracks[req.params.id!]?.find((t) => t.id === req.params.trackId)
    if (!track) return notFound('track not found')
    const file = req.body instanceof FormData ? req.body.get('file') : null
    if (file instanceof Blob && !['image/jpeg', 'image/png', 'image/webp'].includes(file.type)) {
      return apiError(422, 'unsupported image format')
    }
    try {
      track.artworkUrl = await imageDataUrl(file, 300 * 1024)
    } catch (e) {
      return apiError(e instanceof TooLargeError ? 413 : 400, e instanceof Error ? e.message : 'missing file')
    }
    track.artworkStatus = 'manual'
    track.artworkSource = 'manual'
    return json({ track })
  })

  r.on('POST', '/api/v1/tracklists/:id/generate-image', (req, c) => {
    const tl = find(c, req.params.id!)
    if (!tl) return notFound('tracklist not found')
    const format = req.query.get('format') || 'story'
    if (!['story', 'square', 'both'].includes(format)) return apiError(400, 'invalid format: must be story, square, or both')
    const formats = format === 'both' ? (['story', 'square'] as const) : [format as 'story' | 'square']
    const tracks = c.state.tracks[tl.id] ?? []
    const djName = String(c.state.settings.dj_name || 'Sam Example')
    const out: Record<string, string> = {}
    for (const f of formats) {
      const svg = trackcardSvg({ title: tl.title, djName, format: f, tracks })
      out[f] = c.objectUrl(new Blob([svg], { type: 'image/svg+xml' }))
    }
    return json(out)
  })
}
