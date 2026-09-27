// Mirrors the P2.3 door contract (.claude/plans/promoter-p2-guests-door.plan.md,
// "P2.3 contract (offline door)"). JSON is snake_case; times are RFC 3339 UTC.
import type { GuestStatus, ImportPreset, ListType, PriceMode, TicketStatus } from '~/types/guest'

export type SubjectKind = 'guest' | 'ticket'
export type Direction = 'in' | 'out'
export type CounterKind = 'walkup' | 'in' | 'out'

export interface DoorSubject {
  kind: SubjectKind
  id: string
}

export interface DoorEvent {
  id: string
  title: string
  starts_at: string
  ends_at: string
  doors_at: string | null
  timezone: string
  capacity: number | null
}

export interface DoorEntryTerms {
  price_mode: PriceMode
  reduced_price_text: string
  cutoff_at: string | null
  perks: string[]
}

export interface DoorList {
  id: string
  name: string
  type: ListType
  entry_terms: DoorEntryTerms
}

/** Name-only: no email or phone ever reaches the door. */
export interface DoorGuest {
  id: string
  list_id: string
  name: string
  plus_n: number
  status: GuestStatus
  note: string
}

export interface DoorTicket {
  id: string
  name: string
  ticket_type: string
  order_ref: string
  source: ImportPreset
  status: TicketStatus
  /** The QR payload, so scans match offline. */
  secret: string
}

export interface DoorCheckin {
  nonce: string
  subject: DoorSubject
  count: number
  direction: Direction
  at: string
  device_id: string
  undone: boolean
  conflict: boolean
}

export interface DoorCounters {
  walkups: number
  manual_in: number
  manual_out: number
}

/** PBKDF2-SHA256(pin, salt, iterations) → 32 bytes; base64 (standard). */
export interface ManagerPinVerifier {
  salt: string
  iterations: number
  hash: string
}

export interface DoorBundle {
  generated_at: string
  device_id: string
  session_expires_at: string
  event: DoorEvent
  lists: DoorList[]
  guests: DoorGuest[]
  tickets: DoorTicket[]
  checkins: DoorCheckin[]
  counters: DoorCounters
  cursor: string
  manager_pin: ManagerPinVerifier | null
}

// ---------------------------------------------------------------- sync

export interface CheckinOp {
  nonce: string
  type: 'checkin'
  subject: DoorSubject
  count: number
  direction: Direction
  at: string
}

export interface UndoOp {
  nonce: string
  type: 'undo'
  /** Nonce of the check-in or counter op to undo. */
  target: string
  at: string
}

export interface CounterOp {
  nonce: string
  type: 'counter'
  kind: CounterKind
  delta: number
  at: string
}

export type DoorOp = CheckinOp | UndoOp | CounterOp

export interface SyncRequest {
  since: string | null
  ops: DoorOp[]
}

export type OpStatus = 'applied' | 'duplicate' | 'rejected'

export interface OpResult {
  nonce: string
  status: OpStatus
  conflict?: boolean
  error?: string
}

export interface SyncResponse {
  results: OpResult[]
  /** All devices, received after `since`. */
  checkins: DoorCheckin[]
  counters: DoorCounters
  cursor: string
}

export interface DoorAdd {
  /** Client uuid; becomes the guest id so a check-in can be queued before sync. */
  id: string
  nonce: string
  list_id: string
  name: string
  plus_n: number
  manager_pin: string
  at: string
}

export interface AddResult {
  nonce: string
  id: string
  status: OpStatus
  error?: string
}

export interface AddsResponse {
  results: AddResult[]
}

// ---------------------------------------------------------------- staff side

/** What this browser keeps in localStorage['klubhub-door-device']. */
export interface DoorDeviceRecord {
  id: string
  label: string
  token: string
  event: { id: string, title: string, starts_at: string }
}

export interface DoorDevice {
  id: string
  label: string
  created_at: string
  last_seen_at: string | null
  revoked_at: string | null
}

/** POST /api/v1/door/devices (the token is shown once). */
export interface RegisteredDevice {
  id: string
  label: string
  token: string
}

export interface PinInput {
  manager: boolean
  valid_until: string
}

export interface PinResult {
  pin: string
  valid_until: string
  manager?: boolean
}

/** A live PIN window; locked_until is set while too many wrong tries lock it. */
export interface PinWindow {
  valid_until: string
  locked_until?: string | null
}

export interface PinStatus {
  staff: PinWindow | null
  manager: PinWindow | null
}

// ---------------------------------------------------------------- device-side state

/** A rejected op or add, kept to tell staff after a sync. */
export interface DoorRejection {
  nonce: string
  what: string
  error: string
  at: string
}

/** Own ops (synced or not), so an undo knows what it takes back. */
export type JournalEntry = (CheckinOp | CounterOp) & { synced: boolean, undone?: boolean }

/** The undo toast after a door action. */
export interface DoorToast {
  id: number
  text: string
  /** The op to take back; null for plain notices. */
  nonce: string | null
  tone?: 'ok' | 'warn'
}
