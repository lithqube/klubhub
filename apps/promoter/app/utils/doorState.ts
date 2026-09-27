/**
 * Door state derivations (P2.3). Pure: the store keeps the server snapshot
 * (bundle check-ins, counters, cursor) and the local op queue; everything
 * the door shows is derived here, so an offline device and a synced one
 * agree on heads, remaining allowance and occupancy.
 */
import type {
  CounterOp, DoorBundle, DoorCheckin, DoorCounters, DoorGuest, DoorList, DoorOp, DoorSubject, DoorTicket, JournalEntry, OpResult,
} from '~/types/door'

export const subjectKey = (s: DoorSubject) => `${s.kind}:${s.id}`

export interface LocalCheckin extends DoorCheckin {
  /** Still in the local queue (not acknowledged by the server). */
  pending: boolean
}

/**
 * Server check-ins overlaid with the queued ops, in queue order. A queued
 * check-in whose nonce the server already has (the response was lost) is
 * not counted twice; a queued undo marks its target undone.
 */
export function effectiveCheckins(server: DoorCheckin[], queue: DoorOp[], deviceId: string): LocalCheckin[] {
  const byNonce = new Map<string, LocalCheckin>()
  for (const c of server) byNonce.set(c.nonce, { ...c, pending: false })
  for (const op of queue) {
    if (op.type === 'checkin') {
      if (!byNonce.has(op.nonce)) {
        byNonce.set(op.nonce, {
          nonce: op.nonce, subject: op.subject, count: op.count, direction: op.direction, at: op.at, device_id: deviceId,
          undone: false, conflict: false, pending: true,
        })
      }
    } else if (op.type === 'undo') {
      const t = byNonce.get(op.target)
      if (t) byNonce.set(op.target, { ...t, undone: true })
    }
  }
  return [...byNonce.values()]
}

/** Merge a sync delta into the server snapshot: same nonce → replaced, new → appended; ordered by `at`. */
export function mergeCheckins(server: DoorCheckin[], delta: DoorCheckin[]): DoorCheckin[] {
  const byNonce = new Map(server.map(c => [c.nonce, c]))
  for (const c of delta) byNonce.set(c.nonce, c)
  return [...byNonce.values()].sort((a, b) => a.at.localeCompare(b.at))
}

/** Heads inside per subject: non-undone `in` minus `out` counts. */
export function headsBySubject(checkins: DoorCheckin[]): Map<string, number> {
  const m = new Map<string, number>()
  for (const c of checkins) {
    if (c.undone) continue
    const k = subjectKey(c.subject)
    m.set(k, (m.get(k) ?? 0) + (c.direction === 'in' ? c.count : -c.count))
  }
  return m
}

/** Subjects with a server-flagged conflict (double entry across devices) on a live row. */
export function conflictedSubjects(checkins: DoorCheckin[]): Set<string> {
  return new Set(checkins.filter(c => c.conflict && !c.undone).map(c => subjectKey(c.subject)))
}

/** Heads a subject may bring: guest 1 + plus_n, ticket 1. */
export function allowance(subject: DoorSubject, guests: DoorGuest[]): number {
  if (subject.kind === 'ticket') return 1
  const g = guests.find(x => x.id === subject.id)
  return g ? 1 + g.plus_n : 1
}

/** The list's entry cutoff has passed. */
export function pastCutoff(list: Pick<DoorList, 'entry_terms'> | null | undefined, now: number): boolean {
  const c = list?.entry_terms.cutoff_at
  return !!c && Date.parse(c) <= now
}

export type Block = 'cancelled' | 'refunded'

export interface SubjectView {
  key: string
  subject: DoorSubject
  name: string
  guest: DoorGuest | null
  ticket: DoorTicket | null
  list: DoorList | null
  allowance: number
  heads: number
  remaining: number
  /** More heads inside than the allowance. */
  over: boolean
  /** The server flagged a double entry from another device. */
  conflict: boolean
  pastCutoff: boolean
  /** A ticket that cannot enter, with why. */
  blocked: Block | null
  /** Not blocking, but worth a second look (guest not "going", ticket unpaid). */
  warning: string | null
  /** The latest live `in` for this subject (any device), for "ALREADY IN · 23:12 · DOOR 2". */
  lastIn: { at: string, device_id: string } | null
}

function latestIn(checkins: DoorCheckin[], key: string): { at: string, device_id: string } | null {
  let best: DoorCheckin | null = null
  for (const c of checkins) {
    if (c.undone || c.direction !== 'in' || subjectKey(c.subject) !== key) continue
    if (!best || c.at > best.at) best = c
  }
  return best ? { at: best.at, device_id: best.device_id } : null
}

type BundleLike = Pick<DoorBundle, 'guests' | 'tickets' | 'lists'>

export function subjectView(b: BundleLike, checkins: DoorCheckin[], subject: DoorSubject, now: number): SubjectView | null {
  const key = subjectKey(subject)
  const heads = Math.max(0, headsBySubject(checkins.filter(c => subjectKey(c.subject) === key)).get(key) ?? 0)
  const conflict = conflictedSubjects(checkins).has(key)
  const lastIn = heads > 0 ? latestIn(checkins, key) : null
  if (subject.kind === 'guest') {
    const g = b.guests.find(x => x.id === subject.id)
    if (!g) return null
    const list = b.lists.find(l => l.id === g.list_id) ?? null
    const allow = 1 + g.plus_n
    return {
      key, subject, name: g.name, guest: g, ticket: null, list, allowance: allow, heads, remaining: Math.max(0, allow - heads),
      over: heads > allow, conflict, pastCutoff: pastCutoff(list, now), blocked: null,
      warning: g.status === 'going' ? null : `ON THE LIST AS ${g.status.toUpperCase()}`, lastIn,
    }
  }
  const t = b.tickets.find(x => x.id === subject.id)
  if (!t) return null
  const blocked: Block | null = t.status === 'cancelled' || t.status === 'refunded' ? t.status : null
  return {
    key, subject, name: t.name || `Order ${t.order_ref}`, guest: null, ticket: t, list: null, allowance: 1, heads,
    remaining: Math.max(0, 1 - heads), over: heads > 1, conflict, pastCutoff: false, blocked,
    warning: t.status === 'pending' ? 'TICKET UNPAID' : null, lastIn,
  }
}

const bump = (c: DoorCounters, op: Pick<CounterOp, 'kind' | 'delta'>, sign: 1 | -1): DoorCounters => {
  const d = op.delta * sign
  if (op.kind === 'walkup') return { ...c, walkups: c.walkups + d }
  if (op.kind === 'in') return { ...c, manual_in: c.manual_in + d }
  return { ...c, manual_out: c.manual_out + d }
}

/**
 * Server counters plus queued counter ops (unless undone in the queue),
 * minus synced counter ops that a queued undo takes back.
 */
export function effectiveCounters(server: DoorCounters, queue: DoorOp[], journal: JournalEntry[]): DoorCounters {
  const undone = new Set(queue.filter(o => o.type === 'undo').map(o => (o as { target: string }).target))
  const queued = new Set(queue.map(o => o.nonce))
  let c = { ...server }
  for (const op of queue) {
    if (op.type === 'counter' && !undone.has(op.nonce)) c = bump(c, op, 1)
  }
  for (const j of journal) {
    if (j.type === 'counter' && j.synced && !queued.has(j.nonce) && undone.has(j.nonce)) c = bump(c, j, -1)
  }
  return c
}

export type OccupancyTone = 'ok' | 'near' | 'full'

export interface Occupancy {
  inside: number
  capacity: number | null
  /** 0–100, null without capacity. */
  pct: number | null
  tone: OccupancyTone
}

/** Heads inside: checked-in heads (in − out) + walk-ups + manual in − manual out, against capacity. */
export function occupancy(checkins: DoorCheckin[], counters: DoorCounters, capacity: number | null): Occupancy {
  let heads = 0
  for (const n of headsBySubject(checkins).values()) heads += Math.max(0, n)
  const inside = Math.max(0, heads + counters.walkups + counters.manual_in - counters.manual_out)
  if (!capacity) return { inside, capacity: null, pct: null, tone: 'ok' }
  const pct = Math.min(100, Math.round((inside / capacity) * 100))
  return { inside, capacity, pct, tone: inside >= capacity ? 'full' : pct >= 90 ? 'near' : 'ok' }
}

/**
 * Queue an undo. A target still waiting in the queue (and not in flight)
 * is simply dropped — nothing reaches the server; otherwise an undo op is
 * queued for the server to apply.
 */
export function queueUndo(queue: DoorOp[], target: string, inflight: ReadonlySet<string>, nonce: string, at: string): DoorOp[] {
  const i = queue.findIndex(o => o.nonce === target && o.type !== 'undo')
  if (i >= 0 && !inflight.has(target)) return queue.filter((_, k) => k !== i)
  if (queue.some(o => o.type === 'undo' && o.target === target)) return queue
  return [...queue, { nonce, type: 'undo', target, at }]
}

export interface Settled {
  queue: DoorOp[]
  /** Nonces the server applied (or had already). */
  acked: Set<string>
  rejected: OpResult[]
}

/** Drop every op the server answered (applied, duplicate or rejected) from the queue. */
export function settle(queue: DoorOp[], results: OpResult[]): Settled {
  const answered = new Map(results.map(r => [r.nonce, r]))
  const acked = new Set(results.filter(r => r.status !== 'rejected').map(r => r.nonce))
  return {
    queue: queue.filter(o => !answered.has(o.nonce)),
    acked,
    rejected: results.filter(r => r.status === 'rejected'),
  }
}

export type ReasonCode = 'already_in' | 'no_heads' | 'conflict' | 'warning' | 'cutoff'

/** Why a card needs a second look before ADMIT, most important first. */
export function admitReasons(v: Pick<SubjectView, 'remaining' | 'heads' | 'conflict' | 'warning' | 'pastCutoff'>): { code: ReasonCode, text: string }[] {
  const r: { code: ReasonCode, text: string }[] = []
  if (v.remaining === 0) r.push(v.heads ? { code: 'already_in', text: 'ALREADY IN' } : { code: 'no_heads', text: 'NO HEADS LEFT' })
  if (v.conflict) r.push({ code: 'conflict', text: 'DOUBLE ENTRY FLAGGED' })
  if (v.warning) r.push({ code: 'warning', text: v.warning })
  if (v.pastCutoff) r.push({ code: 'cutoff', text: 'PAST CUTOFF' })
  return r
}

/** A confirming tap this soon after arming is a double tap, not a decision. */
export const ARM_GUARD_MS = 700

/** The armed ADMIT may fire: armed, and not within ARM_GUARD_MS of arming. */
export function confirmAllowed(armedAt: number | null, now: number, guard = ARM_GUARD_MS): boolean {
  return armedAt !== null && now - armedAt >= guard
}

/** Short device label from a device id: this device, else the id's last 4 characters. */
export function deviceTag(deviceId: string, self: string): string {
  return deviceId === self ? 'THIS DOOR' : `DOOR ${deviceId.replace(/-/g, '').slice(-4).toUpperCase()}`
}

export interface RecentEntry {
  nonce: string
  /** Guest or ticket name, or WALK-UP / OUT / IN for counters. */
  name: string
  /** "2 IN" for a check-in, "+1" / "−1" for a counter. */
  what: string
  at: string
  synced: boolean
}

/** The last own door actions (newest first) that can still be undone. */
export function recentEntries(journal: JournalEntry[], b: Pick<DoorBundle, 'guests' | 'tickets'> | null, limit = 10): RecentEntry[] {
  const out: RecentEntry[] = []
  for (let i = journal.length - 1; i >= 0 && out.length < limit; i--) {
    const j = journal[i]!
    if (j.undone) continue
    if (j.type === 'counter') {
      const name = j.kind === 'walkup' ? 'WALK-UP' : j.kind === 'out' ? 'OUT' : 'IN'
      out.push({ nonce: j.nonce, name, what: `${j.kind === 'out' ? '−' : '+'}${j.delta}`, at: j.at, synced: j.synced })
      continue
    }
    const s = j.subject
    const name = (s.kind === 'guest' ? b?.guests.find(g => g.id === s.id)?.name : b?.tickets.find(t => t.id === s.id)?.name) || 'Guest'
    out.push({ nonce: j.nonce, name, what: `${j.count} ${j.direction === 'in' ? 'IN' : 'OUT'}`, at: j.at, synced: j.synced })
  }
  return out
}

/** Minutes before the door session ends that the door starts warning. */
export const SESSION_WARN_MIN = 30

/** The door session ends within SESSION_WARN_MIN minutes (and has not ended yet). */
export function sessionEndingSoon(b: Pick<DoorBundle, 'session_expires_at'>, now: number, warnMin = SESSION_WARN_MIN): boolean {
  const left = Date.parse(b.session_expires_at) - now
  return left > 0 && left <= warnMin * 60_000
}

/** Hours after the event end until the door cache is wiped. */
export const EVENT_GRACE_HOURS = 6

/** Why the door cache must be wiped now, or null. */
export function expiryReason(b: Pick<DoorBundle, 'session_expires_at' | 'event'>, now: number): 'session' | 'event' | null {
  if (Date.parse(b.session_expires_at) <= now) return 'session'
  if (Date.parse(b.event.ends_at) + EVENT_GRACE_HOURS * 3_600_000 <= now) return 'event'
  return null
}

/** The API refuses PIN windows longer than this. */
export const MAX_PIN_WINDOW_HOURS = 36

/**
 * Default end of a door PIN window: event end + 6 h, capped at 36 h from
 * now (minus a minute of slack), and at least an hour ahead. Rounded down
 * to the minute.
 */
export function defaultPinValidUntil(endsAt: string, now: number): Date {
  const cap = now + MAX_PIN_WINDOW_HOURS * 3_600_000 - 60_000
  const want = Date.parse(endsAt) + EVENT_GRACE_HOURS * 3_600_000
  const ms = Math.min(Math.max(want, now + 3_600_000), cap)
  return new Date(Math.floor(ms / 60_000) * 60_000)
}

const REJECTION_TEXT: Record<string, string> = {
  unknown_subject: 'no longer on this event’s list',
  unknown_target: 'nothing to undo on the server',
  unknown_list: 'the list no longer exists',
  id_conflict: 'someone with the same id already exists',
  manager_pin_invalid: 'manager PIN not accepted (wrong, expired or locked)',
  event_full: 'the event is full',
  event_purged: 'guest names for this event were erased',
}

/** Plain words for a server rejection code ("invalid_op: …" and "invalid_add: …" match by prefix). */
export function rejectionText(code: string): string {
  if (REJECTION_TEXT[code]) return REJECTION_TEXT[code]
  const m = /^invalid_(?:op|add):\s*(.*)$/.exec(code)
  if (m) return m[1] ? `not valid (${m[1]})` : 'not valid'
  return code.replaceAll('_', ' ')
}
