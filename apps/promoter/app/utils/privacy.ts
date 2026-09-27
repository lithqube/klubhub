/**
 * Pure helpers for privacy and retention (P2.5): retention validation,
 * purge dates and banner copy, erased-row counts and error copy.
 * No Vue, no fetch.
 */
import type { EventPrivacy, PurgeTrigger, RetentionPreview } from '~/types/privacy'
import { instantToZoned } from '~/utils/datetime'

const DAY_MS = 86_400_000

export const RETENTION_MIN = 1
export const RETENTION_MAX = 365
export const RETENTION_DEFAULT = 30
export const RETENTION_PRESETS = [7, 30, 90, 365] as const

/** Shown wherever a personal-data action is off because the event was erased. */
export const PURGED_REASON = 'Guest names and contacts for this event were erased, so this is no longer possible.'

/** What an erase removes and what survives: one list for the settings and the erase dialog. */
export const ERASED_ITEMS = [
  'Guest names, emails, phone numbers and notes',
  'Ticket holder and buyer names, emails and ticket barcodes',
  'Submitter contacts and the event\'s door PINs',
] as const
export const KEPT_ITEMS = [
  'How many came: counts, statuses and check-ins (shown as "Erased guest")',
  'The report: check-in curve, walk-ups, numbers by list and submitter',
  'Lists, entry terms and submitter names',
] as const

/** Retention days typed by the organiser: a whole number from 1 to 365. */
export function validateRetentionDays(input: string | number): { days: number, error: null } | { days: null, error: string } {
  const s = String(input).trim()
  const bad = { days: null, error: `Enter a whole number of days from ${RETENTION_MIN} to ${RETENTION_MAX}.` } as const
  if (!/^\d{1,4}$/.test(s)) return bad
  const n = Number(s)
  return n >= RETENTION_MIN && n <= RETENTION_MAX ? { days: n, error: null } : bad
}

export const daysLabel = (n: number) => `${n} ${n === 1 ? 'day' : 'days'}`

/**
 * "12 Nov" in tz; "12 Nov 2027" when the date is not in the current year
 * (the year of `now` in the same timezone).
 */
export function shortDate(iso: string, tz: string, now = Date.now()): string {
  const d = instantToZoned(iso, tz).date
  const thisYear = instantToZoned(new Date(now).toISOString(), tz).date.slice(0, 4)
  const [y, m, day] = d.split('-').map(Number) as [number, number, number]
  const month = new Intl.DateTimeFormat('en-GB', { month: 'short', timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, 1)))
  return `${day} ${month}${String(y) === thisYear ? '' : ` ${y}`}`
}

/** Whole days between the event end and its purge date (rounded, never negative). */
export function daysAfterEnd(endsAt: string, purgeAfter: string): number {
  return Math.max(0, Math.round((Date.parse(purgeAfter) - Date.parse(endsAt)) / DAY_MS))
}

/**
 * "30 days after it ended" / "1 day after it ended" / "the day it ended";
 * "… after it ends" / "the day it ends" for an event that has not ended.
 */
export function daysAfterText(n: number, ended = true): string {
  const verb = ended ? 'ended' : 'ends'
  return n === 0 ? `the day it ${verb}` : `${daysLabel(n)} after it ${verb}`
}

/** The purge date: the server's purge_after, else ends_at + retention. */
export function purgeDate(endsAt: string, p: Pick<EventPrivacy, 'purge_after' | 'retention_days'>): string {
  return p.purge_after ?? new Date(Date.parse(endsAt) + p.retention_days * DAY_MS).toISOString()
}

export type BannerState = 'none' | 'scheduled' | 'due' | 'purged'

export interface PrivacyBanner {
  state: BannerState
  text: string
  /** Purge date (scheduled/due) or purge time (purged). */
  at: string | null
}

/**
 * The quiet guests/report banner: nothing before the event ends; then when
 * names and contacts are erased; "due" once the date has passed and the
 * hourly job has not run yet; then when they were erased.
 */
export function privacyBanner(event: { ends_at: string, timezone: string }, p: EventPrivacy | null, now = Date.now()): PrivacyBanner {
  if (!p) return { state: 'none', text: '', at: null }
  if (p.purged_at) {
    return { state: 'purged', text: `Guest names and contacts were erased on ${shortDate(p.purged_at, event.timezone, now)}.`, at: p.purged_at }
  }
  if (Date.parse(event.ends_at) > now) return { state: 'none', text: '', at: null }
  const at = purgeDate(event.ends_at, p)
  const when = `${shortDate(at, event.timezone, now)} (${daysAfterText(daysAfterEnd(event.ends_at, at))})`
  if (Date.parse(at) <= now) {
    return { state: 'due', text: `Guest names and contacts for this event are being erased now (scheduled for ${shortDate(at, event.timezone, now)}, ${daysAfterText(daysAfterEnd(event.ends_at, at))}).`, at }
  }
  return { state: 'scheduled', text: `Guest names and contacts for this event are erased on ${when}.`, at }
}

const COUNT_LABELS: [key: string, one: string, many: string][] = [
  ['guests', 'guest', 'guests'],
  ['order_positions', 'ticket', 'tickets'],
  ['orders', 'order', 'orders'],
  ['guest_allocations', 'submitter contact', 'submitter contacts'],
  ['door_pins', 'door PIN', 'door PINs'],
]

/** "12 guests · 4 tickets · 1 order" (zeros left out); "nothing to erase" when empty. */
export function countsText(counts: Record<string, number> | null | undefined): string {
  const c = counts ?? {}
  const parts: string[] = []
  for (const [key, one, many] of COUNT_LABELS) {
    const n = Number(c[key] ?? 0)
    if (n > 0) parts.push(`${n} ${n === 1 ? one : many}`)
  }
  const known = new Set(COUNT_LABELS.map(([k]) => k))
  const other = Object.entries(c).filter(([k]) => !known.has(k)).reduce((n, [, v]) => n + (Number(v) || 0), 0)
  if (other > 0) parts.push(`${other} other ${other === 1 ? 'row' : 'rows'}`)
  return parts.length ? parts.join(' · ') : 'nothing to erase'
}

export const TRIGGER_LABEL: Record<PurgeTrigger, string> = { schedule: 'AUTOMATIC', manual: 'ERASED BY HAND' }

/** Refusals that ask for a fresh sign-in before an erase. */
export function isStepUp(code: string): boolean {
  return code === 'reauthentication_required' || code === 'step_up_required' || code === 'recent_auth_required'
}

/** The policy wants a sign-in in the last 15 minutes; the UI asks again after 14 so a slow form never races it. */
export const STEP_UP_FRESH_MS = 14 * 60_000

/** The sign-in is too old for an erase (unknown or unreadable times are left to the server). */
export function authIsStale(authTime: string | null | undefined, now = Date.now()): boolean {
  const t = Date.parse(authTime ?? '')
  return Number.isFinite(t) && now - t > STEP_UP_FRESH_MS
}

export type StepUpWhy = 'erase' | 'retention'

/** SIGN IN AGAIN: the login page, then straight back to `next` (a path with its own query). */
export function signInAgainRoute(next: string, why: StepUpWhy) {
  return { path: '/login', query: { next, why } }
}

/** The login page's line when it was opened to confirm an erase. */
export function stepUpLoginText(why: unknown): string {
  if (why === 'erase') return 'Confirm it\'s you to erase guest data.'
  if (why === 'retention') return 'Confirm it\'s you to change how long guest data is kept.'
  return ''
}

/** Plain-language copy for ERASE NOW refusals. */
export function purgeErrorText(err: { error: string, field?: string }): string {
  if (isStepUp(err.error)) return 'For safety, erasing needs a recent sign-in (in the last 15 minutes). Sign in again and you\'ll come straight back here.'
  switch (err.error) {
    case 'event_not_ended': return 'This event has not ended yet. Guest data can only be erased after the night.'
    case 'event_purged': return 'Guest names and contacts for this event were already erased.'
    case 'invalid': case 'confirm_mismatch': return 'The title you typed does not match the event title.'
    case 'forbidden': case 'no_role_grant': return 'Only owners and admins can erase guest data.'
    case 'not_found': return 'This event no longer exists.'
    default: return 'Could not erase. Check your connection and try again.'
  }
}

/** Plain-language copy for retention save refusals. */
export function retentionErrorText(err: { error: string, problem?: string }): string {
  if (isStepUp(err.error)) return 'For safety, erasing guest data needs a recent sign-in (in the last 15 minutes). Sign in again and you\'ll come straight back here.'
  switch (err.error) {
    case 'invalid': return `Enter a whole number of days from ${RETENTION_MIN} to ${RETENTION_MAX}.`
    case 'retention_would_purge': return 'The events this would erase changed. Check the list above, then save again.'
    case 'forbidden': case 'no_role_grant': return 'Only owners and admins can change how long guest data is kept.'
    default: return 'Could not save the retention period. Check your connection and try again.'
  }
}

/** Titles shown in the shorten-retention warning before "…". */
const WARN_TITLES = 3

/**
 * The warning before saving a shorter period that erases ended events at
 * once: "With 7 days, guest names and contacts of 3 ended events (A, B,
 * C, …) are erased within the hour. This can't be undone." Empty when
 * nothing would be erased.
 */
export function wouldPurgeText(days: number, preview: Pick<RetentionPreview, 'would_purge' | 'count'> | null): string {
  const n = preview?.count ?? 0
  if (!preview || n <= 0) return ''
  const titles = preview.would_purge.slice(0, WARN_TITLES).map(w => w.title)
  const more = n > titles.length ? ', …' : ''
  const list = titles.length ? ` (${titles.join(', ')}${more})` : ''
  return `With ${daysLabel(days)}, guest names and contacts of ${n} ended ${n === 1 ? 'event' : 'events'}${list} are erased within the hour. This can't be undone.`
}

/**
 * The retention SAVE button: "SAVE AND ERASE N" in the error style while a
 * shorter period erases ended events, enabled only once "I understand" is
 * ticked and the sign-in is recent; off while the preview for a shorter
 * value is still loading.
 */
export function retentionSaveButton(s: {
  busy: boolean, changed: boolean, shorter: boolean, previewing: boolean, erasing: number, ack: boolean, stale: boolean
}): { label: string, danger: boolean, disabled: boolean } {
  const danger = s.erasing > 0
  const label = s.busy ? 'SAVING…' : danger ? `SAVE AND ERASE ${s.erasing}` : 'SAVE RETENTION'
  const disabled = s.busy || !s.changed || (s.shorter && s.previewing) || (danger && (!s.ack || s.stale))
  return { label, danger, disabled }
}

const WS = /[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+/
const DASHES = /[\u2010-\u2015\u2212\ufe58\ufe63\uff0d]/g
const SINGLE_QUOTES = /[\u2018-\u201b\u2032]/g
const DOUBLE_QUOTES = /[\u201c-\u201f\u2033]/g

/**
 * How a typed confirmation is compared with the event title, the same as
 * the server (retention.NormalizeTitle): NFKC, dashes to '-', curly quotes
 * to straight ones, lower case, runs of whitespace to one space, trimmed.
 */
export function normalizeTitle(s: string): string {
  return s.normalize('NFKC').replace(DASHES, '-').replace(SINGLE_QUOTES, '\'').replace(DOUBLE_QUOTES, '"').toLowerCase()
    .split(WS).filter(Boolean).join(' ')
}

/** The title typed in the erase dialog matches (after normalizeTitle; an empty title never matches). */
export function confirmMatches(typed: string, title: string): boolean {
  const want = normalizeTitle(title)
  return want !== '' && normalizeTitle(typed) === want
}
