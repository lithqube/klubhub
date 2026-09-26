/**
 * Pure helpers for the guest table (P2.1): paste parsing, email extraction,
 * search folding, counts, cutoff times and CSV cells. No Vue, no fetch.
 */
import type { Guest, GuestCounts, GuestStatus, ListType } from '~/types/guest'
import { instantToZoned, zonedToInstant } from '~/utils/datetime'

export const MAX_PLUS_N = 10

export const STATUSES: GuestStatus[] = ['going', 'pending', 'waitlist', 'invited', 'declined']

export const STATUS_LABEL: Record<GuestStatus, string> = {
  going: 'GOING', pending: 'PENDING', waitlist: 'WAITLIST', invited: 'INVITED', declined: 'DECLINED',
}

export const LIST_TYPES: { id: ListType, label: string }[] = [
  { id: 'artist', label: 'ARTIST' }, { id: 'promoter', label: 'PROMOTER' }, { id: 'comp', label: 'COMP' },
  { id: 'industry', label: 'INDUSTRY' }, { id: 'vip', label: 'VIP' }, { id: 'reduced', label: 'REDUCED' },
  { id: 'crew', label: 'CREW' },
]

const EMAIL_RE = /[^\s<>(),;"']+@[^\s<>(),;"']+\.[a-z]{2,}/gi
const PLUS_RE = /[\s,]*[([]?\+\s*(\d{1,2})\s*[)\]]?\s*$/
const BULLET_RE = /^\s*(?:[-•*·]|\d{1,3}[.)])\s+/

export interface PastedGuest {
  name: string
  plus_n: number
  /** An email found on the line (dropped for name-only lists). */
  email: string
}

/**
 * Parse pasted lines like "Mara Weiss +2", "- Tom (+1)", "3. Ines" or
 * "Lena W <lena@example.org> +1" into guests. Blank lines and lines with
 * no name are skipped.
 */
export function parsePastedGuests(text: string): PastedGuest[] {
  const out: PastedGuest[] = []
  for (const raw of text.split(/\r?\n/)) {
    let line = raw.replace(BULLET_RE, '')
    let plus = 0
    const m = line.match(PLUS_RE)
    if (m) {
      plus = Number(m[1])
      line = line.slice(0, m.index)
    }
    const email = line.match(EMAIL_RE)?.[0] ?? ''
    if (email) line = line.replace(email, ' ').replace(/<\s*>|\(\s*\)|\[\s*\]/g, ' ')
    const name = line.replace(/\s+/g, ' ').replace(/^[\s,;]+|[\s,;]+$/g, '')
    if (!name) continue
    out.push({ name: name.slice(0, 120), plus_n: plus, email })
  }
  return out
}

/** Unique emails in pasted text, in order (case-insensitive), as typed. */
export function parseEmails(text: string): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const m of text.match(EMAIL_RE) ?? []) {
    const k = m.toLowerCase()
    if (!seen.has(k)) {
      seen.add(k)
      out.push(m)
    }
  }
  return out
}

const FOLD: Record<string, string> = { ł: 'l', đ: 'd', ø: 'o', æ: 'ae', œ: 'oe', ß: 'ss', þ: 'th', ı: 'i' }

/** Case-, accent- and whitespace-insensitive form (mirrors envelope.NormalizeName). */
export function fold(s: string): string {
  return s.toLowerCase().normalize('NFKD').replace(/\p{Mn}/gu, '')
    .replace(/[łđøæœßþı]/g, c => FOLD[c] ?? c).replace(/\s+/g, ' ').trim()
}

/** Guests whose name, email or note contain every word of q. */
export function searchGuests(guests: Guest[], q: string): Guest[] {
  const words = fold(q).split(' ').filter(Boolean)
  if (!words.length) return guests
  return guests.filter((g) => {
    const hay = fold(`${g.name} ${g.email} ${g.note}`)
    return words.every(w => hay.includes(w))
  })
}

/** Tab counts for a set of guests (what the server returns, computed locally). */
export function countByStatus(guests: Guest[]): GuestCounts {
  const c: GuestCounts = { all: 0, going: 0, pending: 0, waitlist: 0, invited: 0, declined: 0, going_heads: 0 }
  for (const g of guests) {
    c.all++
    c[g.status]++
    if (g.status === 'going') c.going_heads += 1 + g.plus_n
  }
  return c
}

/** Statuses that hold allocation quota. */
export const holdsQuota = (s: GuestStatus) => s === 'going' || s === 'pending' || s === 'invited'

/** Heads (guest + N) a set of guests puts on an allocation. */
export function headsHeld(guests: Pick<Guest, 'status' | 'plus_n'>[]): number {
  return guests.reduce((n, g) => n + (holdsQuota(g.status) ? 1 + g.plus_n : 0), 0)
}

export type FillTone = 'ok' | 'near' | 'full'

/** Quota bar: percentage (0–100) and tone. */
export function quotaFill(used: number, quota: number): { pct: number, tone: FillTone } {
  if (quota <= 0) return { pct: 0, tone: 'ok' }
  const pct = Math.min(100, Math.round((used / quota) * 100))
  return { pct, tone: used >= quota ? 'full' : pct >= 80 ? 'near' : 'ok' }
}

function addDays(date: string, days: number): string {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}

/**
 * Instant for a local cutoff ("01:00") on a night: the first occurrence at
 * or after 12 hours before the start, in the event's timezone (mirrors
 * guest.CutoffFor on the server).
 */
export function cutoffInstant(startsIso: string, tz: string, hhmm: string): string {
  const from = new Date(Date.parse(startsIso) - 12 * 3_600_000)
  const day = instantToZoned(from.toISOString(), tz).date
  let c = zonedToInstant(day, hhmm, tz)
  if (c < from) c = zonedToInstant(addDays(day, 1), hhmm, tz)
  return c.toISOString()
}

/** Where an allocation stands for new guests. */
export function allocationState(a: { revoked_at: string | null, deadline: string | null }, now = Date.now()): 'open' | 'closed' | 'revoked' {
  if (a.revoked_at) return 'revoked'
  if (a.deadline && Date.parse(a.deadline) < now) return 'closed'
  return 'open'
}

/**
 * CSV cell safe against formula injection: cells starting with = + - @,
 * tab or CR get an apostrophe; quotes, commas and newlines are quoted.
 * Mirrors guest.SafeCell + encoding/csv on the server.
 */
export function csvCell(v: string | number): string {
  let s = String(v)
  if (s && '=+-@\t\r'.includes(s[0]!)) s = `'${s}`
  return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

export function toCsv(rows: (string | number)[][]): string {
  return rows.map(r => r.map(csvCell).join(',')).join('\n') + '\n'
}

/** Plain-language copy for guest API errors (quota, deadline, revoke, name-only…). */
export function guestErrorText(err: { error: string, field?: string, problem?: string, detail?: Record<string, unknown> }): string {
  const d = err.detail ?? {}
  switch (err.error) {
    case 'quota_exceeded': {
      const free = Math.max(0, Number(d.quota) - Number(d.used))
      return `${String(d.label ?? 'This allocation')} is at ${Number(d.used)} of ${Number(d.quota)} heads. `
        + `These guests need ${Number(d.requested)}; ${free === 0 ? 'none are left' : `only ${free} left`}. Raise the quota or waitlist them.`
    }
    case 'allocation_closed': return 'The deadline for this allocation has passed. Extend it to add more guests.'
    case 'allocation_revoked': return 'This allocation was revoked and takes no new guests.'
    case 'list_not_empty': return `This list still has ${Number(d.guests)} guests.`
    case 'not_found': return 'That list or guest no longer exists. Reload to see the latest.'
    case 'invalid': return err.problem ? `${(err.field ?? '').replace(/^guests\[(\d+)\]\./, (_, i) => `Line ${Number(i) + 1}: `).replace('plus_n', '+N').replace(/_/g, ' ')} — ${err.problem}` : 'Check the highlighted field.'
    case 'forbidden': case 'no_role_grant': return 'Your role cannot change guest lists.'
    default: return 'Could not save. Check your connection and try again.'
  }
}
