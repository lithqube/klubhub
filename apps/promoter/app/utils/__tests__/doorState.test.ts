import { describe, expect, it } from 'vitest'
import type { DoorCheckin, DoorGuest, DoorList, DoorOp, DoorTicket, JournalEntry } from '~/types/door'
import {
  allowance, conflictedSubjects, defaultPinValidUntil, effectiveCheckins, effectiveCounters, expiryReason, headsBySubject, mergeCheckins, occupancy,
  pastCutoff, queueUndo, rejectionText, settle, subjectKey, subjectView,
} from '../doorState'

const NOW = Date.parse('2026-10-03T23:30:00Z')
const at = (min: number) => new Date(NOW + min * 60_000).toISOString()

const lists: DoorList[] = [
  { id: 'l-artist', name: 'Artist guests', type: 'artist', entry_terms: { price_mode: 'free', reduced_price_text: '', cutoff_at: at(-30), perks: ['drink token'] } },
  { id: 'l-comp', name: 'Comp', type: 'comp', entry_terms: { price_mode: 'free', reduced_price_text: '', cutoff_at: null, perks: [] } },
]
const guests: DoorGuest[] = [
  { id: 'g1', list_id: 'l-artist', name: 'Mara Weiss', plus_n: 2, status: 'going', note: '' },
  { id: 'g2', list_id: 'l-comp', name: 'Sam Oduya', plus_n: 0, status: 'invited', note: 'plus-one TBC' },
]
const tickets: DoorTicket[] = [
  { id: 't1', name: 'Hana Kim', ticket_type: 'Early bird', order_ref: 'D-7001', source: 'dice', status: 'valid', secret: 'DICE-0001' },
  { id: 't2', name: 'Theo Brandt', ticket_type: 'Regular', order_ref: 'D-7002', source: 'dice', status: 'refunded', secret: 'DICE-0003' },
]
const bundle = { lists, guests, tickets }

const row = (nonce: string, id: string, count: number, extra: Partial<DoorCheckin> = {}): DoorCheckin => ({
  nonce, subject: { kind: id.startsWith('t') ? 'ticket' : 'guest', id }, count, direction: 'in', at: at(0), device_id: 'd-a',
  undone: false, conflict: false, ...extra,
})
const op = (nonce: string, id: string, count = 1): DoorOp => ({
  nonce, type: 'checkin', subject: { kind: id.startsWith('t') ? 'ticket' : 'guest', id }, count, direction: 'in', at: at(1),
})

describe('heads and allowance', () => {
  it('counts non-undone in minus out per subject', () => {
    const m = headsBySubject([
      row('a', 'g1', 2), row('b', 'g1', 1, { undone: true }), row('c', 'g1', 1, { direction: 'out' }), row('d', 't1', 1),
    ])
    expect(m.get('guest:g1')).toBe(1)
    expect(m.get('ticket:t1')).toBe(1)
  })

  it('allows a guest 1 + plus_n and a ticket 1', () => {
    expect(allowance({ kind: 'guest', id: 'g1' }, guests)).toBe(3)
    expect(allowance({ kind: 'guest', id: 'g2' }, guests)).toBe(1)
    expect(allowance({ kind: 'ticket', id: 't1' }, guests)).toBe(1)
  })
})

describe('effectiveCheckins', () => {
  it('overlays queued check-ins and undos on the server rows', () => {
    const server = [row('s1', 'g1', 1)]
    const queue: DoorOp[] = [op('q1', 'g1', 1), { nonce: 'u1', type: 'undo', target: 's1', at: at(2) }]
    const eff = effectiveCheckins(server, queue, 'd-me')
    expect(eff.find(c => c.nonce === 's1')).toMatchObject({ undone: true, pending: false })
    expect(eff.find(c => c.nonce === 'q1')).toMatchObject({ pending: true, device_id: 'd-me', count: 1 })
    expect(headsBySubject(eff).get('guest:g1')).toBe(1)
    expect(server[0]!.undone).toBe(false) // inputs untouched
  })

  it('does not count a queued op twice when the server already has its nonce', () => {
    const eff = effectiveCheckins([row('n1', 'g1', 2)], [op('n1', 'g1', 2)], 'd-a')
    expect(eff).toHaveLength(1)
    expect(headsBySubject(eff).get('guest:g1')).toBe(2)
  })
})

describe('mergeCheckins', () => {
  it('replaces by nonce, appends new rows and keeps time order', () => {
    const merged = mergeCheckins(
      [row('a', 'g1', 1, { at: at(0) }), row('b', 't1', 1, { at: at(5) })],
      [row('b', 't1', 1, { at: at(5), conflict: true }), row('c', 'g2', 1, { at: at(3), device_id: 'd-b' })],
    )
    expect(merged.map(c => c.nonce)).toEqual(['a', 'c', 'b'])
    expect(merged.find(c => c.nonce === 'b')!.conflict).toBe(true)
  })
})

describe('subjectView', () => {
  it('shows allowance, heads, remaining and the list cutoff for a guest', () => {
    const v = subjectView(bundle, [row('a', 'g1', 2)], { kind: 'guest', id: 'g1' }, NOW)!
    expect(v).toMatchObject({ name: 'Mara Weiss', allowance: 3, heads: 2, remaining: 1, over: false, pastCutoff: true, blocked: null, warning: null })
    expect(v.list?.name).toBe('Artist guests')
  })

  it('warns about guests not marked going and flags over-admission', () => {
    const v = subjectView(bundle, [row('a', 'g2', 1), row('b', 'g2', 1)], { kind: 'guest', id: 'g2' }, NOW)!
    expect(v.warning).toBe('ON THE LIST AS INVITED')
    expect(v).toMatchObject({ heads: 2, remaining: 0, over: true, pastCutoff: false })
  })

  it('blocks refunded tickets and reports server conflicts', () => {
    expect(subjectView(bundle, [], { kind: 'ticket', id: 't2' }, NOW)!.blocked).toBe('refunded')
    const v = subjectView(bundle, [row('a', 't1', 1, { conflict: true })], { kind: 'ticket', id: 't1' }, NOW)!
    expect(v).toMatchObject({ blocked: null, conflict: true, remaining: 0 })
    expect(conflictedSubjects([row('a', 't1', 1, { conflict: true, undone: true })]).size).toBe(0)
    expect(subjectView(bundle, [], { kind: 'guest', id: 'nope' }, NOW)).toBeNull()
  })
})

describe('pastCutoff', () => {
  it('compares the list cutoff with now', () => {
    expect(pastCutoff(lists[0], NOW)).toBe(true)
    expect(pastCutoff(lists[0], NOW - 3_600_000)).toBe(false)
    expect(pastCutoff(lists[1], NOW)).toBe(false)
    expect(pastCutoff(null, NOW)).toBe(false)
  })
})

describe('counters and occupancy', () => {
  const server = { walkups: 10, manual_in: 2, manual_out: 5 }

  it('adds queued counters, skips ones undone in the queue, subtracts undone synced ones', () => {
    const queue: DoorOp[] = [
      { nonce: 'w1', type: 'counter', kind: 'walkup', delta: 1, at: at(0) },
      { nonce: 'w2', type: 'counter', kind: 'walkup', delta: 1, at: at(0) },
      { nonce: 'o1', type: 'counter', kind: 'out', delta: 1, at: at(0) },
      { nonce: 'u1', type: 'undo', target: 'w2', at: at(1) },
      { nonce: 'u2', type: 'undo', target: 's-w', at: at(1) },
    ]
    const journal: JournalEntry[] = [{ nonce: 's-w', type: 'counter', kind: 'walkup', delta: 1, at: at(-5), synced: true }]
    expect(effectiveCounters(server, queue, journal)).toEqual({ walkups: 10, manual_in: 2, manual_out: 6 })
  })

  it('occupancy = heads + walk-ups + in − out, against capacity', () => {
    const checkins = [row('a', 'g1', 3), row('b', 't1', 1), row('c', 'g2', 1, { undone: true })]
    expect(occupancy(checkins, server, 100)).toEqual({ inside: 11, capacity: 100, pct: 11, tone: 'ok' })
    expect(occupancy(checkins, server, 12).tone).toBe('near')
    expect(occupancy(checkins, server, 11)).toMatchObject({ tone: 'full', pct: 100 })
    expect(occupancy(checkins, server, null)).toEqual({ inside: 11, capacity: null, pct: null, tone: 'ok' })
    expect(occupancy([], { walkups: 0, manual_in: 0, manual_out: 3 }, 10).inside).toBe(0)
  })
})

describe('queueUndo', () => {
  it('drops a still-queued target instead of sending an undo', () => {
    const q = [op('a', 'g1'), op('b', 'g1')]
    expect(queueUndo(q, 'a', new Set(), 'u', at(0)).map(o => o.nonce)).toEqual(['b'])
  })

  it('queues an undo when the target is synced or in flight, once', () => {
    const q = [op('a', 'g1')]
    const inflight = queueUndo(q, 'a', new Set(['a']), 'u1', at(0))
    expect(inflight.map(o => o.nonce)).toEqual(['a', 'u1'])
    const synced = queueUndo([], 's1', new Set(), 'u2', at(0))
    expect(synced).toEqual([{ nonce: 'u2', type: 'undo', target: 's1', at: at(0) }])
    expect(queueUndo(synced, 's1', new Set(), 'u3', at(0))).toBe(synced)
  })
})

describe('settle', () => {
  it('drops answered ops and reports rejections', () => {
    const q = [op('a', 'g1'), op('b', 'g1'), op('c', 'g1')]
    const s = settle(q, [
      { nonce: 'a', status: 'applied' }, { nonce: 'b', status: 'duplicate' }, { nonce: 'x', status: 'rejected', error: 'unknown_subject' },
    ])
    expect(s.queue.map(o => o.nonce)).toEqual(['c'])
    expect([...s.acked]).toEqual(['a', 'b'])
    expect(s.rejected).toEqual([{ nonce: 'x', status: 'rejected', error: 'unknown_subject' }])
  })
})

describe('expiryReason', () => {
  const event = { id: 'e', title: 'x', starts_at: at(-120), ends_at: at(360), doors_at: null, timezone: 'Europe/Berlin', capacity: null }
  it('wipes at session expiry or 6 h after the event end', () => {
    expect(expiryReason({ session_expires_at: at(60), event }, NOW)).toBeNull()
    expect(expiryReason({ session_expires_at: at(-1), event }, NOW)).toBe('session')
    expect(expiryReason({ session_expires_at: at(10_000), event }, NOW + (360 + 6 * 60) * 60_000)).toBe('event')
  })
})

describe('subjectKey', () => {
  it('prefixes the kind', () => {
    expect(subjectKey({ kind: 'ticket', id: 'x' })).toBe('ticket:x')
  })
})

describe('defaultPinValidUntil', () => {
  const h = 3_600_000
  it('is event end + 6 h when that is within 36 h', () => {
    expect(defaultPinValidUntil(new Date(NOW + 8 * h).toISOString(), NOW).getTime()).toBe(NOW + 14 * h)
  })
  it('is capped at 36 h from now for a later event', () => {
    const d = defaultPinValidUntil(new Date(NOW + 6 * 24 * h).toISOString(), NOW).getTime()
    expect(d).toBeLessThanOrEqual(NOW + 36 * h)
    expect(d).toBeGreaterThan(NOW + 35.9 * h)
  })
  it('is at least an hour ahead after the event', () => {
    expect(defaultPinValidUntil(new Date(NOW - 24 * h).toISOString(), NOW).getTime()).toBe(NOW + h)
  })
})

describe('rejectionText', () => {
  it('names known codes and matches validation errors by prefix', () => {
    expect(rejectionText('manager_pin_invalid')).toContain('manager PIN')
    expect(rejectionText('invalid_op: count 1 to 50')).toBe('not valid (count 1 to 50)')
    expect(rejectionText('invalid_add:')).toBe('not valid')
    expect(rejectionText('something_new')).toBe('something new')
  })
})
