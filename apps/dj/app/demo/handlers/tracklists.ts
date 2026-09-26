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

  r.on('DELETE', '/api/v1/tracklists/:id', (req, c) => {
    const i = c.state.tracklists.findIndex((t) => t.id === req.params.id)
    if (i === -1) return notFound('tracklist not found')
    c.state.tracklists.splice(i, 1)
    Reflect.deleteProperty(c.state.tracks, req.params.id!)
    for (const links of Object.values(c.state.gigLinks)) links.tracklists = links.tracklists.filter((t) => t !== req.params.id)
    return noContent()
  })

  r.on('PUT', '/api/v1/tracklists/:id/tracks/:trackId', (req, c) => {
    const track = c.state.tracks[req.params.id!]?.find((t) => t.id === req.params.trackId)
    if (!track) return notFound('track not found')
    const body = (req.body ?? {}) as Record<string, unknown>
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
