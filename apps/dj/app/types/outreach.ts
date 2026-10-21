// Mirrors the Go API (api/internal/outreach, mounted at /api/v1/outreach).
// Times are ISO-8601 instants; slot dates are calendar dates (YYYY-MM-DD).
// The DJ side is single-user OSS: suppressions store the email in plaintext
// (consistent with contacts.email) — unlike the promoter side's blind index.

export interface AvailabilitySlot {
  id: string
  /** Calendar date (YYYY-MM-DD). */
  date: string
  city: string
  country: string
  genres: string[]
  fee_min?: number | null
  fee_max?: number | null
  currency?: string | null
  notes: string
  created_at: string
}

export interface SlotCreate {
  date: string
  city: string
  country: string
  genres: string[]
  fee_min: number | null
  fee_max: number | null
  currency: string | null
  notes: string
}

/** A promoter contact matched to a slot (SPEC §8). Contacts without an
 * email are still returned, with has_email=false — matchable but not
 * sendable (OUTR-05 v2.1). */
export interface MatchedContact {
  id: string
  name: string
  company?: string
  email: string
  has_email: boolean
  city: string
  /** A past gig in the slot's city (join key: gigs.promoter_email) — ranks first. */
  past_gig_in_city: boolean
}

/** A venue in the slot's city, surfaced as context. */
export interface MatchedVenue {
  id: string
  name: string
  city: string
}

/** The match result for one availability slot. */
export interface SlotMatch {
  slot: AvailabilitySlot
  contacts: MatchedContact[]
  venues: MatchedVenue[]
}

export interface OutreachTemplate {
  id: string
  name: string
  direction: 'dj_to_promoter'
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

export type RequestStatus = 'queued' | 'sending' | 'sent' | 'failed' | 'bounced'

/** One row of the sent log: a booking-request email, linked to the email
 * outbox row for delivery status. */
export interface OutreachRequest {
  id: string
  contact_id: string
  availability_slot_id?: string | null
  template_id?: string | null
  epk_export_id?: string | null
  email_message_id?: string | null
  to_email: string
  subject: string
  body: string
  status: RequestStatus
  sent_at?: string | null
  created_at: string
  email_status?: string
  email_error?: string
}

export interface SendRequestInput {
  contact_id: string
  availability_slot_id?: string | null
  template_id?: string | null
  epk_export_id?: string | null
  /** Per-request overrides; the server re-renders authoritatively at send. */
  subject?: string | null
  body?: string | null
}

export interface Suppression {
  id: string
  email: string
  reason: string
  created_at: string
}

export interface SuppressionCreate {
  email: string
  reason: string
}

/** GET /api/v1/epk/exports rows (camelCase — the EPK handler's contract). */
export interface EpkExport {
  id: string
  garageObjectKey: string
  createdAt: string
}

/** The DJ's own contact block (user_settings.contact_info) — the source of
 * the {{dj_*}} variables and the ReplyTo of every outreach email
 * (OUTR-06 v2.1). */
export interface DjContactInfo {
  name: string
  email: string
  phone: string
}
