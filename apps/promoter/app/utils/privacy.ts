/**
 * Pure helpers for privacy and retention (P2.5): retention validation,
 * purge dates and banner copy, erased-row counts and error copy.
 * No Vue, no fetch.
 */
import type { EventPrivacy, PurgeTrigger } from '~/types/privacy'
import { instantToZoned } from '~/utils/datetime'

const DAY_MS = 86_400_000

export const RETENTION_MIN = 1
export const RETENTION_MAX = 365
export const RETENTION_DEFAULT = 30
export const RETENTION_PRESETS = [7, 30, 90, 365] as const

/** Shown wherever a personal-data action is off because the event was erased. */
export const PURGED_REASON = 'Guest names and contacts for this event were erased, so this is no longer possible.'

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

/** "30 days after it ended" / "1 day after it ended" / "the day it ended". */
export function daysAfterText(n: number): string {
  return n === 0 ? 'the day it ended' : `${daysLabel(n)} after it ended`
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
    return { state: 'due', text: `Guest names and contacts for this event are due to be erased: ${when}. This happens within the hour.`, at }
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

/** Plain-language copy for ERASE NOW refusals. */
export function purgeErrorText(err: { error: string, field?: string }): string {
  if (isStepUp(err.error)) return 'For safety, erasing needs a recent sign-in (in the last 15 minutes). Sign in again, then erase.'
  switch (err.error) {
    case 'mfa_required': return 'Erasing needs two-factor sign-in. Set up an authenticator app, then sign in again.'
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
  switch (err.error) {
    case 'invalid': return `Enter a whole number of days from ${RETENTION_MIN} to ${RETENTION_MAX}.`
    case 'forbidden': case 'no_role_grant': return 'Only owners and admins can change how long guest data is kept.'
    case 'mfa_required': return 'Changing retention needs two-factor sign-in. Set up an authenticator app, then sign in again.'
    default: return 'Could not save the retention period. Check your connection and try again.'
  }
}

/** The title typed in the erase dialog matches (surrounding spaces ignored, case and inner text exact). */
export const confirmMatches = (typed: string, title: string) => typed.trim() === title.trim() && title.trim() !== ''
