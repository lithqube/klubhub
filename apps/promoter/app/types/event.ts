// Mirrors the Go API (api/internal/promoter/event). Times are ISO-8601 instants.

export type EventStatus = 'draft' | 'scheduled' | 'published' | 'cancelled' | 'postponed'
export type Visibility = 'public' | 'unlisted' | 'private'
export type LocationMode = 'venue' | 'city_only' | 'secret'
export type EventView = 'upcoming' | 'drafts' | 'past'

export interface PromoterEvent {
  id: string
  title: string
  slug: string
  status: EventStatus
  visibility: Visibility
  publish_at: string | null
  starts_at: string
  ends_at: string
  doors_at: string | null
  timezone: string
  venue_id: string | null
  city: string
  location_mode: LocationMode
  location_reveal_at: string | null
  min_age: number | null
  genres: string[]
  description_md: string
  cost_text: string
  external_ticket_url: string | null
  capacity: number | null
  version: number
  created_at: string
  updated_at: string
}

export interface Stage {
  id: string
  name: string
  position: number
  curfew_at: string | null
  changeover_minutes: number
}

export interface LineupEntry {
  id: string
  stage_id: string | null
  display_name: string
  profile_url: string | null
  billing_order: number
  b2b_group: number | null
  set_start: string | null
  set_end: string | null
}

export type IssueCode =
  | 'overlap' | 'outside_event' | 'past_curfew' | 'no_stage' | 'unknown_stage' | 'b2b_mismatch'
  | 'short_changeover' | 'dead_air' | 'untimed'

export interface Issue {
  code: IssueCode
  severity: 'error' | 'warning'
  stage_id?: string
  entry_ids?: string[]
  from?: string
  to?: string
  minutes?: number
}

export interface VenueRef { id: string, name: string, city: string }

export interface EventDetail extends PromoterEvent {
  venue: VenueRef | null
  stages: Stage[]
  lineup: LineupEntry[]
  issues: Issue[]
}

export interface EventSummary extends PromoterEvent {
  venue_name: string | null
  act_count: number
  stage_count: number
  untimed_count: number
  /** Timetable issue counts; 0 on past events. */
  error_count: number
  warning_count: number
}

export interface EventInput {
  title: string
  slug?: string
  starts_at: string
  ends_at: string
  doors_at: string | null
  timezone: string
  venue_id: string | null
  city: string
  location_mode: LocationMode
  location_reveal_at: string | null
  visibility: Visibility
  publish_at: string | null
  min_age: number | null
  genres: string[]
  description_md: string
  cost_text: string
  external_ticket_url: string | null
  capacity: number | null
}

export interface StageInput { id?: string, name: string, curfew_at: string | null, changeover_minutes: number }

export interface LineupInput {
  id?: string
  stage_id: string | null
  display_name: string
  profile_url: string | null
  b2b_group: number | null
  set_start: string | null
  set_end: string | null
}

export interface Room { id: string, name: string, capacity: number | null, position: number }

export interface Venue {
  id: string
  name: string
  city: string
  country: string | null
  timezone: string
  capacity: number | null
  curfew_local: string | null
  tech_notes: string
  rooms: Room[]
  protected: { address: boolean, geo: boolean, contact: boolean }
  archived_at: string | null
}

export interface VenueProtected {
  address: string
  geo: string
  contact_name: string
  contact_email: string
  contact_phone: string
}

export interface VenueInput {
  name: string
  city: string
  country: string | null
  timezone: string
  capacity: number | null
  curfew_local: string | null
  tech_notes: string
  rooms: { id?: string, name: string, capacity: number | null }[]
  protected?: VenueProtected | null
}

export interface PublicLocation {
  name: string
  city: string
  country?: string
  address?: string
  withheld: boolean
  reveal_at?: string
}

/** The collective as exports show it (P1.5). */
export interface OrganizerProfile {
  name: string
  url?: string
  same_as?: string[]
  bio?: string
  accent_color?: string
}

export interface ExportData {
  event: PromoterEvent
  lineup: LineupEntry[]
  stages: Stage[]
  location: PublicLocation
  organizer: string
  organizer_profile: OrganizerProfile
  embargoed: boolean
}

/** Normalised API error (see api/internal/promoter/event/handler.go). */
export interface ApiError {
  error: string
  field?: string
  problem?: string
  issues?: Issue[]
  entries?: string[]
  status?: number
}
