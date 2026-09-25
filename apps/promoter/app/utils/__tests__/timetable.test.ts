import { describe, expect, it } from 'vitest'
import type { LineupEntry, Stage } from '~/types/event'
import { hasErrors, validateTimetable } from '../timetable'

// Same fixtures as api/internal/promoter/event/timetable_test.go (Berlin, CEST = UTC+2).
const at = (day: number, h: number, m = 0) => new Date(Date.UTC(2026, 9, day, h - 2, m)).toISOString()
const ev = { starts_at: at(3, 23), ends_at: at(4, 7), doors_at: at(3, 22, 30) }
const main: Stage = { id: 'main', name: 'Main Room', position: 0, curfew_at: at(4, 5), changeover_minutes: 15 }
const garden: Stage = { id: 'garden', name: 'Garden', position: 1, curfew_at: null, changeover_minutes: 10 }
let n = 0
const set = (stage: string | null, start: string | null, end: string | null, extra: Partial<LineupEntry> = {}): LineupEntry =>
  ({ id: `e${n++}`, stage_id: stage, display_name: 'x', profile_url: null, billing_order: 0, b2b_group: null, set_start: start, set_end: end, ...extra })
const codes = (list: { code: string }[]) => list.reduce<Record<string, number>>((m, i) => ({ ...m, [i.code]: (m[i.code] ?? 0) + 1 }), {})

describe('validateTimetable (parity with Go)', () => {
  it('accepts a clean overnight timetable', () => {
    expect(validateTimetable(ev, [main, garden], [
      set('main', at(3, 23), at(4, 1)), set('main', at(4, 1, 15), at(4, 2, 45)), set('main', at(4, 3), at(4, 5)),
      set('garden', at(3, 23), at(4, 1, 30)),
    ])).toEqual([])
  })

  it('reports errors and warnings like the server', () => {
    const issues = validateTimetable(ev, [main, garden], [
      set('main', at(4, 0, 30), at(4, 2, 30)), set('main', at(4, 2), at(4, 5, 30)),
      set('garden', at(3, 21), at(3, 23)), set('garden', at(4, 1), at(4, 2)), set('garden', at(4, 2, 5), at(4, 3)),
      set(null, null, null), set('ghost', at(4, 1), at(4, 2)),
    ])
    expect(codes(issues)).toEqual({ overlap: 1, past_curfew: 1, outside_event: 1, dead_air: 1, short_changeover: 1, untimed: 1, unknown_stage: 1 })
    expect(issues.find(i => i.code === 'overlap')?.minutes).toBe(30)
    expect(hasErrors(issues)).toBe(true)
  })

  it('treats a b2b pair as one slot and flags mismatched partners', () => {
    const a = set('main', at(4, 1), at(4, 3), { b2b_group: 1 })
    const b = set('main', at(4, 1), at(4, 3), { b2b_group: 1 })
    expect(codes(validateTimetable(ev, [main], [a, b]))).toEqual({})
    b.set_end = at(4, 3, 30)
    expect(codes(validateTimetable(ev, [main], [a, b]))).toEqual({ b2b_mismatch: 1 })
  })
})
