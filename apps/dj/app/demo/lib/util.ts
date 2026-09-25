// Small helpers for the demo handlers: dates, files and calendar export.

import type { Gig } from '../../types/gig'

export const isoDate = (d: Date) => d.toISOString().slice(0, 10)

/** Date `days` from `now` at a given UTC time, as RFC 3339. */
export function daysFrom(now: Date, days: number, hour = 0, minute = 0): string {
  const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() + days, hour, minute))
  return d.toISOString().replace('.000Z', 'Z')
}

/** "2026-10-24T23:45" in `timeZone` → UTC ISO string. */
export function zonedToUtc(local: string, timeZone: string): string {
  const guess = Date.parse(`${local.length === 16 ? local + ':00' : local}Z`)
  if (Number.isNaN(guess)) return new Date().toISOString()
  try {
    const parts = new Intl.DateTimeFormat('en-US', {
      timeZone, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
    }).formatToParts(new Date(guess))
    const get = (t: string) => Number(parts.find((p) => p.type === t)?.value ?? 0)
    const asZone = Date.UTC(get('year'), get('month') - 1, get('day'), get('hour'), get('minute'), get('second'))
    return new Date(guess - (asZone - guess)).toISOString()
  } catch {
    return new Date(guess).toISOString()
  }
}

/** Upload size caps: localStorage holds roughly 5 MB per origin. */
export const MAX_IMAGE_BYTES = 750 * 1024
export const MAX_ASSET_TOTAL = 3 * 1024 * 1024

export class TooLargeError extends Error {}

/** Reads an uploaded image as a data URL, enforcing the demo size cap. */
export async function imageDataUrl(file: unknown, maxBytes = MAX_IMAGE_BYTES): Promise<string> {
  if (!(file instanceof Blob)) throw new TypeError('missing file')
  if (file.size > maxBytes) {
    throw new TooLargeError(`Images are limited to ${Math.round(maxBytes / 1024)} KB in the demo (this browser stores them locally).`)
  }
  const bytes = new Uint8Array(await file.arrayBuffer())
  let bin = ''
  for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  return `data:${file.type || 'application/octet-stream'};base64,${btoa(bin)}`
}

/** Decodes a data URL into a Blob without touching the network. */
export function dataUrlToBlob(url: string): Blob {
  const comma = url.indexOf(',')
  const head = url.slice(0, comma)
  const data = url.slice(comma + 1)
  const type = head.slice(5).split(';')[0] || 'application/octet-stream'
  const bytes = head.includes(';base64')
    ? Uint8Array.from(atob(data), (ch) => ch.charCodeAt(0))
    : new TextEncoder().encode(decodeURIComponent(data))
  return new Blob([bytes], { type })
}

export async function readText(file: unknown): Promise<string> {
  if (file instanceof Blob) return file.text()
  return typeof file === 'string' ? file : ''
}

/** Total size of stored data URLs, in characters (≈ bytes). */
export function assetBytes(assets: Record<string, string>): number {
  return Object.values(assets).reduce((n, v) => n + v.length, 0)
}

const icsText = (s: string) => s.replace(/\\/g, '\\\\').replace(/;/g, '\\;').replace(/,/g, '\\,').replace(/\r?\n/g, '\\n')

/** iCalendar feed for the gig list (all-day events, cancelled flagged). */
export function gigsIcs(gigs: Gig[], stampedAt: Date): string {
  const stamp = stampedAt.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '')
  const lines = ['BEGIN:VCALENDAR', 'VERSION:2.0', 'PRODID:-//KlubHub DJ//Demo//EN', 'CALSCALE:GREGORIAN', 'X-WR-CALNAME:KlubHub DJ gigs (demo)']
  for (const g of gigs) {
    const day = g.date.slice(0, 10).replace(/-/g, '')
    const next = new Date(`${g.date.slice(0, 10)}T00:00:00Z`)
    next.setUTCDate(next.getUTCDate() + 1)
    lines.push(
      'BEGIN:VEVENT',
      `UID:${g.id}@klubhub.example`,
      `DTSTAMP:${stamp}`,
      `DTSTART;VALUE=DATE:${day}`,
      `DTEND;VALUE=DATE:${next.toISOString().slice(0, 10).replace(/-/g, '')}`,
      `SUMMARY:${icsText(g.event_name || g.venue || 'Gig')}`,
      `LOCATION:${icsText([g.venue, g.city, g.country].filter(Boolean).join(', '))}`,
      `STATUS:${g.status === 'cancelled' ? 'CANCELLED' : g.status === 'inquiry' ? 'TENTATIVE' : 'CONFIRMED'}`,
      'END:VEVENT',
    )
  }
  lines.push('END:VCALENDAR')
  return lines.join('\r\n') + '\r\n'
}
