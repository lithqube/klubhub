// Mirrors the Go API (api/internal/promoter/guest). Times are ISO-8601 instants.

export type ListType = 'artist' | 'promoter' | 'comp' | 'industry' | 'vip' | 'reduced' | 'crew'
export type GuestStatus = 'going' | 'pending' | 'waitlist' | 'invited' | 'declined'
export type GuestSource = 'manual' | 'paste' | 'import' | 'door'
export type PriceMode = 'free' | 'reduced'

export interface EntryTerms {
  price_mode: PriceMode
  reduced_price_text: string
  /** Event lists: absolute cutoff. */
  cutoff_at?: string | null
  /** Standing lists: local wall-clock cutoff, "HH:MM". */
  cutoff_local?: string | null
  perks: string[]
}

export interface Allocation {
  id: string
  list_id: string
  label: string
  submitter_contact: string
  quota: number
  plus_n_max: number
  deadline: string | null
  requires_approval: boolean
  revoked_at: string | null
  /** Heads (guest + N) holding quota: going, pending, invited. */
  used: number
  guests: number
  pending: number
}

export interface GuestList {
  id: string
  event_id: string
  name: string
  type: ListType
  entry_terms: EntryTerms
  collect_contact: boolean
  standing_template_id: string | null
  position: number
  allocations: Allocation[]
  guests: number
  heads: number
  pending: number
  /** Sum of active allocation quotas; 0 = no allocations. */
  quota: number
}

export interface StandingList {
  id: string
  name: string
  type: ListType
  entry_terms: EntryTerms
  collect_contact: boolean
  position: number
}

export interface ListInput {
  name: string
  type: ListType
  entry_terms: EntryTerms
  collect_contact: boolean
}

export interface AllocationInput {
  label: string
  /** null leaves the sealed contact unchanged; '' clears it. */
  submitter_contact: string | null
  quota: number
  plus_n_max: number
  deadline: string | null
  requires_approval: boolean
}

export interface Guest {
  id: string
  list_id: string
  allocation_id: string | null
  name: string
  email: string
  phone: string
  note: string
  plus_n: number
  status: GuestStatus
  source: GuestSource
  created_at: string
  updated_at: string
}

export interface GuestCounts {
  all: number
  going: number
  pending: number
  waitlist: number
  invited: number
  declined: number
  going_heads: number
}

export interface GuestPage {
  guests: Guest[]
  counts: GuestCounts
}

export interface GuestInput {
  name: string
  email?: string
  phone?: string
  note?: string
  plus_n: number
  status?: GuestStatus
}

export interface AddGuestsInput {
  list_id: string
  allocation_id: string | null
  source: 'manual' | 'paste'
  guests: GuestInput[]
}

export interface AddResult {
  added: Guest[]
  /** Indexes of inputs skipped as duplicates. */
  duplicates: number[]
}

export interface BulkStatusInput {
  status: GuestStatus
  guest_ids?: string[]
  emails?: string[]
}

export interface BulkResult {
  matched: number
  updated: number
  unmatched: string[]
}

export interface GuestFilter {
  status?: GuestStatus
  list_id?: string
}

export interface OverviewRow {
  event_id: string
  title: string
  status: string
  starts_at: string
  timezone: string
  capacity: number | null
  lists: number
  guests: number
  going_heads: number
  pending: number
  used: number
  quota: number
}
