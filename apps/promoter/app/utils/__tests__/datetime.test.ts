import { describe, expect, it } from 'vitest'
import { instantToZoned, rangeLabel, resolveNight, zonedToInstant } from '../datetime'

describe('datetime', () => {
  it('converts Berlin wall time in summer and winter', () => {
    expect(zonedToInstant('2026-10-03', '23:00', 'Europe/Berlin').toISOString()).toBe('2026-10-03T21:00:00.000Z')
    expect(zonedToInstant('2026-12-05', '23:00', 'Europe/Berlin').toISOString()).toBe('2026-12-05T22:00:00.000Z')
    expect(instantToZoned('2026-10-03T21:00:00.000Z', 'Europe/Berlin')).toEqual({ date: '2026-10-03', time: '23:00' })
  })

  it('rolls an overnight end to the next day and places early doors correctly', () => {
    const n = resolveNight('2026-10-03', '23:00', '07:00', '22:30', 'Europe/Berlin')
    expect(n.overnight).toBe(true)
    expect(n.starts_at).toBe('2026-10-03T21:00:00.000Z')
    expect(n.ends_at).toBe('2026-10-04T05:00:00.000Z')
    expect(n.doors_at).toBe('2026-10-03T20:30:00.000Z')
    expect(n.durationMinutes).toBe(480)
    expect(n.dstShiftMinutes).toBe(0)
    expect(rangeLabel(n.starts_at, n.ends_at, 'Europe/Berlin')).toBe('23:00 → 07:00 +1')
  })

  it('reports the extra hour when the clocks go back (24–25 Oct 2026)', () => {
    const n = resolveNight('2026-10-24', '23:00', '07:00', null, 'Europe/Berlin')
    expect(n.durationMinutes).toBe(540)
    expect(n.dstShiftMinutes).toBe(60)
  })

  it('keeps a same-day event on one day', () => {
    const n = resolveNight('2026-07-12', '14:00', '22:00', null, 'Europe/Berlin')
    expect(n.overnight).toBe(false)
    expect(n.durationMinutes).toBe(480)
  })
})
