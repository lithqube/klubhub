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
  /** Heads inside now: non-undone door `in` counts minus `out` counts, floored at 0 (P2.3 check-ins). */
  heads_in: number
  /** Earliest non-undone `in` (device clock); kept after the guest leaves again. */
  first_in_at: string | null
  /** Personal data erased by retention (P2.5): name, email, phone and note are "". List, status and check-in state stay. */
  purged?: boolean
}

export interface GuestCounts {
  all: number
  going: number
  pending: number
  waitlist: number
  invited: number
  declined: number
  going_heads: number
  /** Valid imported tickets (0 when a list filter is set: tickets are on no list). */
  tickets: number
  /** Guests with heads_in ≥ 1, any status (tickets not included). */
  checked_in: number
}

export interface GuestPage {
  guests: Guest[]
  /** Imported ticket holders; only sent without a status or list filter. */
  tickets: Ticket[]
  counts: GuestCounts
}

// ---------------------------------------------------------------- attendee import (P2.2)

export type ImportPreset = 'ra' | 'dice' | 'shotgun' | 'pretix' | 'luma' | 'generic'
export type TicketStatus = 'valid' | 'pending' | 'cancelled' | 'refunded'
export type ImportField =
  | 'order_ref' | 'ticket_ref' | 'secret' | 'name' | 'first_name' | 'last_name' | 'email'
  | 'buyer_name' | 'buyer_email' | 'ticket_type' | 'ticket_type_ref' | 'status'

/** An imported ticket holder (order position). The barcode is never listed. */
export interface Ticket {
  id: string
  order_id: string
  source: ImportPreset
  order_ref: string
  ticket_type_id: string
  ticket_type: string
  name: string
  email: string
  status: TicketStatus
  imported_at: string
  /** Has a non-undone door `in`. */
  checked_in: boolean
  first_in_at: string | null
  /** Holder name and email erased by retention (P2.5); type, status and check-in state stay. */
  purged?: boolean
}

export interface ImportCounts {
  rows: number
  orders_new: number
  orders_updated: number
  positions_new: number
  positions_updated: number
  positions_unchanged: number
  rejected: number
  /** Tickets imported earlier from the same platform that the file does not mention (left as they are). */
  not_in_file: number
  /** Rows whose email is also on a guest list. */
  on_guest_list: number
}

export interface ImportResult {
  dry_run: boolean
  import_id: string | null
  preset: ImportPreset
  encoding: string
  /** Field → the header it was read from. */
  mapping: Partial<Record<ImportField, string>>
  counts: ImportCounts
  ticket_types: { name: string, new: boolean, tickets: number, valid: number }[]
  /** At most 200; counts.rejected has the total. */
  rejected: { line: number, reason: string }[]
  /** First rows, names and emails masked. */
  preview: { line: number, order_ref: string, name: string, email: string, ticket_type: string, status: TicketStatus, action: 'new' | 'update' | 'unchanged' }[]
}

export interface ImportRequest {
  preset: ImportPreset
  file: Blob
  fileName?: string
  /** Generic preset only: field → column header. */
  mapping?: Partial<Record<ImportField, string>>
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
  /** 'checked_in' (heads_in ≥ 1, any status) works for the guest table, not the CSV export. */
  status?: GuestStatus | 'checked_in'
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
  /** Valid imported tickets. */
  tickets: number
}
