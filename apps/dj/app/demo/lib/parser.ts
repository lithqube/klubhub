// Simple tracklist parser for the demo upload flow. Understands rekordbox
// XML (TRACK attributes), tab/comma-separated exports with a header row,
// and plain "Artist - Title" lines (numbering and timestamps are ignored).

import type { ParseWarning } from '../../types/tracklist'

export interface ParsedTrack {
  title: string
  artist: string
  album: string
  genre: string
  bpm: number
  durationSecs: number
  musicalKey: string
  rating: number
}

export interface ParseResult {
  sourceFormat: string
  tracks: ParsedTrack[]
  warnings: ParseWarning[]
}

export const MAX_TRACKS = 500

const blank = (): ParsedTrack => ({ title: '', artist: '', album: '', genre: '', bpm: 0, durationSecs: 0, musicalKey: '', rating: 0 })

/** "4:21", "1:02:03", "261" → seconds. */
export function parseDuration(v: string): number {
  const s = v.trim()
  if (!s) return 0
  if (/^\d+(\.\d+)?$/.test(s)) return Math.round(Number(s))
  const parts = s.split(':').map(Number)
  if (parts.some((n) => Number.isNaN(n))) return 0
  return parts.reduce((acc, n) => acc * 60 + n, 0)
}

const num = (v: string | undefined) => {
  const n = Number(String(v ?? '').replace(',', '.'))
  return Number.isFinite(n) ? n : 0
}

function parseXml(text: string): ParseResult {
  const tracks: ParsedTrack[] = []
  for (const m of text.matchAll(/<TRACK\b([^>]*)>/g)) {
    const attrs: Record<string, string> = {}
    for (const a of m[1]!.matchAll(/(\w+)="([^"]*)"/g)) attrs[a[1]!] = decodeEntities(a[2]!)
    if (!attrs.Name) continue // playlist entries only carry a Key
    tracks.push({
      ...blank(),
      title: attrs.Name,
      artist: attrs.Artist ?? '',
      album: attrs.Album ?? '',
      genre: attrs.Genre ?? '',
      bpm: num(attrs.AverageBpm),
      durationSecs: parseDuration(attrs.TotalTime ?? ''),
      musicalKey: attrs.Tonality ?? '',
      rating: Math.round(num(attrs.Rating) / 51),
    })
  }
  return { sourceFormat: 'rekordbox', tracks, warnings: [] }
}

function decodeEntities(s: string): string {
  return s.replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&apos;/g, "'")
}

function splitCsv(line: string): string[] {
  const out: string[] = []
  let cur = ''
  let quoted = false
  for (let i = 0; i < line.length; i++) {
    const c = line[i]
    if (c === '"') {
      if (quoted && line[i + 1] === '"') { cur += '"'; i++ } else quoted = !quoted
    } else if (c === ',' && !quoted) { out.push(cur); cur = '' } else cur += c
  }
  out.push(cur)
  return out
}

const COLUMNS: Record<keyof ParsedTrack, string[]> = {
  title: ['track title', 'title', 'name', 'song'],
  artist: ['artist', 'artists'],
  album: ['album'],
  genre: ['genre'],
  bpm: ['bpm', 'tempo', 'average bpm'],
  durationSecs: ['time', 'length', 'duration'],
  musicalKey: ['key', 'tonality'],
  rating: ['rating'],
}

function parseTable(lines: string[], sep: 'tab' | 'comma'): ParseResult | null {
  const cells = (l: string) => (sep === 'tab' ? l.split('\t') : splitCsv(l)).map((c) => c.trim())
  const header = cells(lines[0]!).map((h) => h.toLowerCase().replace(/^#$/, 'position'))
  const index: Partial<Record<keyof ParsedTrack, number>> = {}
  for (const [field, names] of Object.entries(COLUMNS) as [keyof ParsedTrack, string[]][]) {
    const i = header.findIndex((h) => names.includes(h))
    if (i !== -1) index[field] = i
  }
  if (index.title === undefined) return null
  const tracks: ParsedTrack[] = []
  const warnings: ParseWarning[] = []
  lines.slice(1).forEach((line, i) => {
    const row = cells(line)
    const get = (f: keyof ParsedTrack) => (index[f] === undefined ? '' : row[index[f]!] ?? '')
    const title = get('title')
    if (!title) {
      warnings.push({ row: i + 2, field: 'title', message: 'Row has no title; skipped.' })
      return
    }
    tracks.push({
      ...blank(),
      title,
      artist: get('artist'),
      album: get('album'),
      genre: get('genre'),
      bpm: num(get('bpm')),
      durationSecs: parseDuration(get('durationSecs')),
      musicalKey: get('musicalKey'),
      rating: Math.min(5, get('rating').replace(/[^*\u2605]/g, '').length || num(get('rating'))),
    })
  })
  const fmt = header.includes('track title') ? 'rekordbox' : sep === 'comma' ? 'serato' : 'traktor'
  return { sourceFormat: fmt, tracks, warnings }
}

function parseLines(lines: string[]): ParseResult {
  const tracks: ParsedTrack[] = []
  const warnings: ParseWarning[] = []
  lines.forEach((raw, i) => {
    const line = raw
      .replace(/^\s*\[?\d{1,2}:\d{2}(:\d{2})?\]?\s*/, '')
      .replace(/^\s*\d{1,3}\s*[.):-]\s*/, '')
      .trim()
    if (!line || line.startsWith('#')) return
    const m = /^(.+?)\s+[-\u2013\u2014]\s+(.+)$/.exec(line)
    if (!m) {
      warnings.push({ row: i + 1, field: 'artist', message: 'No "Artist - Title" separator; kept the line as the title.' })
      tracks.push({ ...blank(), title: line })
      return
    }
    const bpm = /\[(\d{2,3}(?:\.\d+)?)\]\s*$/.exec(m[2]!)
    tracks.push({ ...blank(), artist: m[1]!.trim(), title: m[2]!.replace(/\s*\[[\d.]+\]\s*$/, '').trim(), bpm: bpm ? num(bpm[1]) : 0 })
  })
  return { sourceFormat: 'text', tracks, warnings }
}

export function parseTracklist(text: string): ParseResult {
  const clean = (text.charCodeAt(0) === 0xfeff ? text.slice(1) : text).split(String.fromCharCode(0)).join('')
  let result: ParseResult
  if (clean.trimStart().startsWith('<')) result = parseXml(clean)
  else {
    const lines = clean.split(/\r?\n/).filter((l) => l.trim() !== '')
    const first = lines[0] ?? ''
    result = (first.includes('\t') && parseTable(lines, 'tab'))
      || (first.includes(',') && parseTable(lines, 'comma'))
      || parseLines(lines)
  }
  if (result.tracks.length > MAX_TRACKS) {
    result.warnings.push({ row: MAX_TRACKS + 1, field: 'file', message: `Only the first ${MAX_TRACKS} tracks were imported.` })
    result.tracks = result.tracks.slice(0, MAX_TRACKS)
  }
  return result
}
