// Mirrors the Go API (api/internal/promoter/bookings). Times are ISO-8601
// instants. The API returns unsealed values to the authenticated tenant —
// the UI never sees ciphertext (the sealed columns are a storage tier).

export type TalentSource = 'past_gig' | 'referral' | 'manual'
export const TALENT_SOURCES: TalentSource[] = ['past_gig', 'referral', 'manual']

export interface TalentContact {
  id: string
  name: string
  email: string
  phone: string
  /** False when the contact has no email: matchable, never sendable (OFFER-10). */
  has_email: boolean
  city: string
  country: string
  genres: string[]
  source: TalentSource
  notes: string
  created_at: string
  updated_at: string
}

export interface TalentContactInput {
  name: string
  email: string
  phone: string
  city: string
  country: string
  genres: string[]
  source: TalentSource
  notes: string
}

export interface OpenSlot {
  id: string
  event_id: string
  /** May be empty: a general "play this night" slot. */
  role: string
  genres: string[]
  fee_min?: number | null
  fee_max?: number | null
  currency?: string | null
  notes: string
  created_at: string
}

export interface OpenSlotInput {
  role: string
  genres: string[]
  fee_min: number | null
  fee_max: number | null
  currency: string | null
  notes: string
}

export interface OutreachTemplate {
  id: string
  name: string
  direction: 'promoter_to_dj'
  subject: string
  body: string
  is_default: boolean
  version: number
  created_at: string
  updated_at: string
}

export interface TemplateCreate {
  name: string
  subject: string
  body: string
  is_default: boolean
}

export interface TemplateUpdate {
  name?: string
  subject?: string
  body?: string
  is_default?: boolean
  /** Optimistic concurrency: the version the editor loaded (409 on mismatch). */
  version: number
}

export type OfferStatus = 'queued' | 'sending' | 'sent' | 'failed' | 'bounced'

export interface OutreachRequest {
  id: string
  event_id?: string | null
  open_slot_id?: string | null
  talent_contact_id?: string | null
  template_id?: string | null
  email_message_id?: string | null
  to_email: string
  subject: string
  body: string
  status: OfferStatus
  sent_at?: string | null
  created_at: string
  email_status?: string
  email_error?: string
}

export interface SendOfferInput {
  talent_contact_id: string
  open_slot_id?: string | null
  template_id?: string | null
  /** Per-request overrides; the server re-renders authoritatively at send. */
  subject?: string | null
  body?: string | null
}

/** The suppression list stores the email blind index only — the response
 * carries no email; the sent log's recipient is the source for opt-outs. */
export interface Suppression {
  id: string
  reason: string
  created_at: string
}

export interface SuppressionCreate {
  email: string
  reason: string
}

/** One matched talent contact for an open slot, ranked (SPEC §8): genre
 * overlap count desc, then most recently added. */
export interface TalentMatch {
  contact: TalentContact
  overlap_genres: string[]
}
