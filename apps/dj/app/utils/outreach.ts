// Pure helpers for the Outreach page: the client-side request preview
// (the server re-renders authoritatively at send) and the send-error copy.
// Mirrors api/internal/outreach (variables, formatFee, the handler's error
// map). The DJ side is single-user: no tenant scoping, no sealed columns.
import type { AvailabilitySlot, MatchedContact, OutreachRequest, RequestStatus } from '~/types/outreach'

/** Variables the booking-request templates may use (SPEC §7). */
export const OUTREACH_VARIABLES = [
  'dj_name', 'dj_email', 'dj_phone', 'promoter_name', 'promoter_company',
  'city', 'country', 'date', 'genres', 'fee', 'notes', 'epk_link',
] as const

export interface RequestPreviewInput {
  djName: string
  djEmail: string
  djPhone: string
  contact: MatchedContact | null
  slot: AvailabilitySlot | null
  epkLink: string
}

/** Formats a money amount like the Go formatMoney: two decimals, trailing
 * zeros and the dot trimmed ("800", "800.5"). */
export function formatMoneyAmount(v: number): string {
  return v.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')
}

/** Formats a slot fee the way formatFee does: "500 EUR", "500–800 EUR",
 * "from 500 EUR", "up to 800 EUR", "" when no fee is set. */
export function formatSlotFee(slot: AvailabilitySlot | null): string {
  if (!slot) return ''
  const cur = slot.currency ? ` ${slot.currency}` : ''
  const min = slot.fee_min ?? null
  const max = slot.fee_max ?? null
  if (min != null && max != null && min === max) return `${formatMoneyAmount(min)}${cur}`
  if (min != null && max != null) return `${formatMoneyAmount(min)}–${formatMoneyAmount(max)}${cur}`
  if (min != null) return `from ${formatMoneyAmount(min)}${cur}`
  if (max != null) return `up to ${formatMoneyAmount(max)}${cur}`
  return ''
}

/** Builds the {{var}} map for the preview (mirrors the service's variables). */
export function requestVariables(input: RequestPreviewInput): Record<string, string> {
  const vars: Record<string, string> = {
    dj_name: input.djName,
    dj_email: input.djEmail,
    dj_phone: input.djPhone,
    promoter_name: input.contact?.name ?? '',
    promoter_company: input.contact?.company ?? '',
    epk_link: input.epkLink,
  }
  if (input.slot) {
    vars.city = input.slot.city
    vars.country = input.slot.country
    vars.date = input.slot.date
    vars.genres = input.slot.genres.join(', ')
    vars.fee = formatSlotFee(input.slot)
    vars.notes = input.slot.notes
  }
  return vars
}

/** Tiny {{var}} substitution for the live preview. Unknown variables render
 * empty, like the server's render. */
export function renderRequestText(text: string, vars: Record<string, string>): string {
  return text.replace(/\{\{\s*(\w+)\s*\}\}/g, (_m, name: string) => vars[name] ?? '')
}

/** The $fetch error shape: ofetch carries the status and the response body. */
export interface OutreachApiError {
  statusCode?: number
  status?: number
  data?: { error?: unknown, message?: unknown }
}

/** Send-error copy per the API's status codes (SPEC §12, OUTR-05/06 v2.1).
 * The DJ handler writes {"error": "<human text>"} — the text lives in
 * `error` (validation failures read "validation failed: field: reason; …"). */
export function requestErrorText(e: OutreachApiError | null): string {
  const status = e?.statusCode ?? e?.status
  const code = typeof e?.data?.error === 'string' ? e.data.error : ''
  const message = typeof e?.data?.message === 'string' ? e.data.message : ''
  const text = message || code
  switch (status) {
    case 503:
      return 'Email is not configured (Plunk): booking requests cannot be sent until transactional email is set up.'
    case 409:
      return code === 'conflict'
        ? 'The template changed since you loaded it: reload and try again.'
        : 'This contact is on your do-not-contact list.'
    case 429:
      return 'Daily outreach limit reached: try again tomorrow.'
    case 422:
      // Provider-terms gate: outreach is cold B2B and most hosted providers
      // prohibit it, so the API refuses unless the operator attested a
      // permitted provider. This is an admin configuration step, not a user
      // error — say so plainly rather than echoing the raw server string.
      if (text.includes('provider') || code === 'outreach_not_permitted') {
        return 'Outreach sending is turned off for this server: its email provider\’s terms do not permit unsolicited B2B outreach. An administrator must set OUTREACH_PROVIDER_PERMITS_OUTREACH=true after confirming a permitted provider: see docs/OUTREACH-PROVIDER.md.'
      }
      if (text.includes('no email')) return 'This contact has no email address on file.'
      if (text.includes('no DJ contact email')) {
        return 'Set your contact email in Settings → EPK contact info so promoters can reply.'
      }
      if (text.includes('no garage object key')) return 'This EPK export has no file: export a fresh PDF and try again.'
      return text || 'The booking request was rejected.'
    case 404:
      return 'That contact or slot no longer exists: reload and try again.'
    default:
      return text || 'Could not send the request.'
  }
}

/** Delivery badge tone per status, matching the HUD palette. */
export function requestStatusClass(status: RequestStatus): string {
  switch (status) {
    case 'sent': return 'badge-ready'
    case 'queued':
    case 'sending': return 'badge-draft'
    case 'failed':
    case 'bounced': return 'badge-failed'
    default: return 'badge-archived'
  }
}

/** Renders a slot's calendar date for display (YYYY-MM-DD → e.g. "Fri 2026-11-06"). */
export function formatSlotDate(date: string): string {
  const d = new Date(`${date}T00:00:00Z`)
  if (Number.isNaN(d.getTime())) return date
  const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
  const y = d.getUTCFullYear()
  const m = String(d.getUTCMonth() + 1).padStart(2, '0')
  const day = String(d.getUTCDate()).padStart(2, '0')
  return `${days[d.getUTCDay()]} ${y}-${m}-${day}`
}

/** The genres input value ↔ string[] (comma-separated chips). */
export function genresToText(genres: string[]): string {
  return genres.join(', ')
}

export function textToGenres(text: string): string[] {
  return text.split(',').map(g => g.trim()).filter(Boolean)
}

/** The sent log's delivery label: the outbox status when present, else the
 * request status. */
export function deliveryLabel(r: OutreachRequest): string {
  return (r.email_status || r.status).toUpperCase()
}
