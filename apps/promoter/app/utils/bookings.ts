// Pure helpers for the Bookings page: the client-side offer preview
// (the server re-renders authoritatively at send), the OFFER-09 venue
// withholding rule, and the send-error copy. Mirrors
// api/internal/promoter/bookings (offerVariables, offerVenueName, formatFee).
import type { ApiError, LocationMode, PromoterEvent } from '~/types/event'
import type { OpenSlot, OutreachRequest, TalentContact } from '~/types/bookings'

/** Variables the offer templates may use (SPEC §7). */
export const OFFER_VARIABLES = [
  'org_name', 'promoter_name', 'promoter_email', 'promoter_phone',
  'dj_name', 'event_name', 'date', 'city', 'venue', 'slot_role', 'genres', 'fee', 'notes',
] as const

export interface OfferPreviewInput {
  orgName: string
  promoterName: string
  promoterEmail: string
  contact: TalentContact | null
  event: PromoterEvent | null
  /** EventSummary.venue_name (EventDetail.venue.name) — null without a venue. */
  venueName: string | null
  slot: OpenSlot | null
  now?: Date
}

/** OFFER-09: the offer must never disclose a venue the event withholds
 * from the public — {{venue}} renders empty for 'city_only' and for
 * 'secret' before location_reveal_at (mirrors offerVenueName). */
export function venueWithheld(mode: LocationMode, revealAt: string | null, now = new Date()): boolean {
  if (mode === 'city_only') return true
  if (mode === 'secret') return !revealAt || now.getTime() < Date.parse(revealAt)
  return false
}

/** The venue value the preview (and the server) render for this event. */
export function previewVenue(event: PromoterEvent, venueName: string | null, now = new Date()): string {
  if (venueWithheld(event.location_mode, event.location_reveal_at, now)) return ''
  return venueName ?? ''
}

/** Formats a slot fee the way formatFee does: "500 EUR", "500–800 EUR",
 * "from 500 EUR", "up to 800 EUR", "" when no fee is set. */
export function formatFee(slot: OpenSlot | null): string {
  if (!slot) return ''
  const cur = slot.currency ? ` ${slot.currency}` : ''
  const min = slot.fee_min ?? null
  const max = slot.fee_max ?? null
  if (min != null && max != null && min === max) return `${min}${cur}`
  if (min != null && max != null) return `${min}–${max}${cur}`
  if (min != null) return `from ${min}${cur}`
  if (max != null) return `up to ${max}${cur}`
  return ''
}

/** Renders the event's start in the event's own timezone, like
 * formatEventDate ("2006-01-02 15:04 MST"). */
export function formatEventDate(startsAt: string, timezone: string): string {
  const d = new Date(startsAt)
  try {
    const parts = new Intl.DateTimeFormat('en-GB', {
      timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', hour12: false,
    }).formatToParts(d)
    const get = (t: string) => parts.find(p => p.type === t)?.value ?? ''
    const tz = new Intl.DateTimeFormat('en-GB', { timeZone: timezone, timeZoneName: 'short' })
      .formatToParts(d).find(p => p.type === 'timeZoneName')?.value ?? ''
    return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')} ${tz}`
  } catch {
    return d.toISOString().slice(0, 16).replace('T', ' ')
  }
}

/** Builds the {{var}} map for the preview (mirrors offerVariables). */
export function offerVariables(input: OfferPreviewInput): Record<string, string> {
  const vars: Record<string, string> = {
    org_name: input.orgName,
    promoter_name: input.promoterName,
    promoter_email: input.promoterEmail,
    promoter_phone: '',
    dj_name: input.contact?.name ?? '',
  }
  if (input.event) {
    vars.event_name = input.event.title
    vars.date = formatEventDate(input.event.starts_at, input.event.timezone)
    vars.city = input.event.city
    vars.venue = previewVenue(input.event, input.venueName, input.now)
  }
  if (input.slot) {
    vars.genres = input.slot.genres.join(', ')
    vars.fee = formatFee(input.slot)
    vars.slot_role = input.slot.role
    vars.notes = input.slot.notes
  }
  return vars
}

/** Tiny {{var}} substitution for the live preview. Unknown variables render
 * empty, like the server's render. */
export function renderOfferText(text: string, vars: Record<string, string>): string {
  return text.replace(/\{\{\s*(\w+)\s*\}\}/g, (_m, name: string) => vars[name] ?? '')
}

/** Whether a template or edited text mentions {{venue}} while the event
 * withholds it — the composer shows a warning banner in that case. */
export function mentionsVenue(text: string): boolean {
  return /\{\{\s*venue\s*\}\}/.test(text)
}

/** Send-error copy per the API's error codes (SPEC §12). */
export function offerErrorText(e: ApiError | null): string {
  switch (e?.error) {
    case 'email_unavailable':
      return 'Email is not configured (Plunk): offers cannot be sent until transactional email is set up.'
    // Provider-terms gate: offers are cold B2B outreach and most hosted
    // providers prohibit it, so the API refuses unless an administrator
    // attested a permitted provider. An admin step, not a user error.
    case 'outreach_not_permitted':
      return 'Outreach sending is turned off for this server: its email provider’s terms do not permit unsolicited B2B outreach. An administrator must set OUTREACH_PROVIDER_PERMITS_OUTREACH=true after confirming a permitted provider: see docs/OUTREACH-PROVIDER.md.'
    case 'suppressed':
      return 'This DJ is on your do-not-contact list.'
    case 'conflict':
      return 'The template changed since you loaded it: reload and try again.'
    case 'duplicate':
      return 'This DJ is already on your do-not-contact list.'
    case 'rate_limited':
      return 'Daily offer limit reached: try again tomorrow.'
    case 'no_contact_email':
      return 'This DJ has no email address on file.'
    case 'no_reply_to':
      return 'No reply address for your account: set your email so DJs can reply.'
    case 'forbidden':
      return 'That slot belongs to another event.'
    case 'invalid':
      return e.problem ? `Invalid ${e.field ?? 'input'}: ${e.problem}` : 'The offer was rejected.'
    default:
      return e?.error ? `Could not send the offer (${e.error}).` : 'Could not send the offer.'
  }
}

/** Delivery badge tone per status, matching the HUD palette. */
export function offerStatusClass(status: OutreachRequest['status']): string {
  switch (status) {
    case 'sent': return 'badge-ready'
    case 'queued':
    case 'sending': return 'badge-draft'
    case 'failed':
    case 'bounced': return 'badge-failed'
    default: return 'badge-archived'
  }
}
