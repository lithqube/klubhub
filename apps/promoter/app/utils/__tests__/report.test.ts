import { describe, expect, it } from 'vitest'
import type { CurvePoint, ReportListRow, ReportSubmitterRow } from '~/types/report'
import {
  barPath, bucketAt, bucketLabel, bucketRange, bucketStart, curveGeometry, defaultLayout, formatCount, formatPercent, hasActivity, listBackFilename,
  niceMax, reportPhase, share, sortLists, sortSubmitters,
} from '../report'

const at = (iso: string) => Date.parse(iso)

describe('formatting', () => {
  it('formats rates, with a dash when there is no denominator', () => {
    expect(formatPercent(0.375)).toBe('38%')
    expect(formatPercent(0)).toBe('0%')
    expect(formatPercent(1)).toBe('100%')
    expect(formatPercent(null)).toBe('—')
    expect(formatPercent(undefined)).toBe('—')
    expect(formatPercent(Number.NaN)).toBe('—')
    expect(share(3, 4)).toBe(0.75)
    expect(share(3, 0)).toBeNull()
    expect(share(3, null)).toBeNull()
    expect(formatCount(1284)).toBe('1,284')
  })

  it('rounds axis maxima to 1, 2 or 5 × 10^k', () => {
    expect([0, 0.4, 1, 3, 7, 12, 20, 21, 480, 1200].map(niceMax)).toEqual([1, 1, 1, 5, 10, 20, 20, 50, 500, 2000])
  })

  it('builds a readable list-back filename', () => {
    expect(listBackFilename('klubnacht-02', 'Kaiser & Friends')).toBe('klubnacht-02-list-back-kaiser-friends.csv')
    expect(listBackFilename('x', 'Sørensen Ä')).toBe('x-list-back-s-rensen-a.csv')
    expect(listBackFilename('x', '***')).toBe('x-list-back-list.csv')
  })
})

describe('buckets in the event timezone', () => {
  it('aligns to local quarter hours', () => {
    // 23:07 in Berlin (CET) → the 23:00 bucket.
    expect(new Date(bucketStart(at('2026-01-10T22:07:00Z'), 'Europe/Berlin')).toISOString()).toBe('2026-01-10T22:00:00.000Z')
    expect(new Date(bucketStart(at('2026-01-10T22:15:00Z'), 'Europe/Berlin')).toISOString()).toBe('2026-01-10T22:15:00.000Z')
    // 05:55 in Kathmandu (+05:45) → the local 05:45 bucket, which is 00:00 UTC.
    expect(new Date(bucketStart(at('2026-01-10T00:10:00Z'), 'Asia/Kathmandu')).toISOString()).toBe('2026-01-10T00:00:00.000Z')
  })

  it('labels a bucket in local time', () => {
    expect(bucketLabel('2026-01-10T22:15:00Z', 'Europe/Berlin')).toBe('23:15')
    expect(bucketRange('2026-01-10T22:45:00Z', 'Europe/Berlin')).toBe('23:45–00:00')
  })
})

describe('report phase and activity', () => {
  const event = { id: 'e', title: 'T', starts_at: '2026-01-10T22:00:00Z', ends_at: '2026-01-11T06:00:00Z', timezone: 'Europe/Berlin', capacity: null }
  it('is LIVE only between start and end', () => {
    expect(reportPhase({ live: true, event }, at('2026-01-10T21:00:00Z'))).toBe('upcoming')
    expect(reportPhase({ live: true, event }, at('2026-01-10T23:00:00Z'))).toBe('live')
    expect(reportPhase({ live: false, event }, at('2026-01-12T00:00:00Z'))).toBe('past')
  })

  it('treats any door count as activity', () => {
    const totals = { heads_admitted: 0, walkups: 0, tickets_scanned: 0 } as never
    expect(hasActivity({ curve: [], totals })).toBe(false)
    expect(hasActivity({ curve: [], totals: { ...(totals as object), walkups: 2 } as never })).toBe(true)
  })
})

describe('table order', () => {
  const list = (list_id: string, type: ReportListRow['type']): ReportListRow => ({
    list_id, name: list_id, type, going: 0, arrived: 0, no_show_rate: null, heads_expected: 0, heads_admitted: 0, plus_ones_allowed: 0, plus_ones_used: 0,
  })
  const sub = (allocation_id: string, list_id: string, list_type: ReportSubmitterRow['list_type'], submitter: string, revoked = false): ReportSubmitterRow => ({
    allocation_id, list_id, list_name: list_id, list_type, submitter, quota: 1, going: 0, arrived: 0, no_show_rate: null, heads_admitted: 0, revoked,
  })

  it('puts artist lists first and keeps the rest in API order', () => {
    expect(sortLists([list('comp', 'comp'), list('art', 'artist'), list('ind', 'industry')]).map(r => r.list_id)).toEqual(['art', 'comp', 'ind'])
  })

  it('puts artist allocations first and otherwise keeps the API order (revoked included)', () => {
    const rows = [
      sub('a1', 'promo', 'promoter', 'Zed'),
      sub('a2', 'art', 'artist', 'Kaiser', true),
      sub('a3', 'art', 'artist', 'Oscar'),
      sub('a4', 'art', 'artist', 'Ben'),
      sub('a5', 'promo', 'promoter', 'Amy'),
    ]
    expect(sortSubmitters(rows).map(r => r.allocation_id)).toEqual(['a2', 'a3', 'a4', 'a1', 'a5'])
  })
})

describe('curve geometry', () => {
  const tz = 'Europe/Berlin'
  const pt = (bucket_start: string, i: number, o: number, walkups: number, occupancy: number): CurvePoint => ({ bucket_start, in: i, out: o, walkups, occupancy })
  const curve = [
    pt('2026-01-10T22:45:00Z', 4, 0, 1, 5),
    pt('2026-01-10T23:00:00Z', 10, 2, 0, 13),
    pt('2026-01-10T23:15:00Z', 0, 3, 0, 10),
  ]
  const l = defaultLayout(400)

  it('uses one scale per panel with clean maxima', () => {
    const g = curveGeometry(curve, tz, l)
    expect(g.occMax).toBe(20)
    expect(g.upMax).toBe(10)
    expect(g.downMax).toBe(5)
    expect(g.occTicks.map(t => t.value)).toEqual([0, 10, 20])
    expect(g.flowTicks.map(t => t.value)).toEqual([10, 0, -5])
  })

  it('draws arrivals above and exits below one baseline, bars at most 24 px', () => {
    const g = curveGeometry(curve, tz, l)
    expect(g.bars).toHaveLength(3)
    for (const b of g.bars) {
      expect(b.w).toBeLessThanOrEqual(24)
      expect(Math.round(b.upY + b.upH)).toBe(Math.round(g.baseline))
      expect(b.downY).toBe(g.baseline)
    }
    expect(g.bars[1]!.upH).toBeCloseTo(l.flowHeight * 10 / 15, 0)
    expect(g.bars[2]!.upH).toBe(0)
    expect(g.bars[2]!.downH).toBeCloseTo(l.flowHeight * 3 / 15, 0)
  })

  it('starts occupancy at zero and plots each value at the bucket end', () => {
    const g = curveGeometry(curve, tz, l)
    expect(g.line.startsWith(`M${l.left},${l.occTop + l.occHeight}`)).toBe(true)
    expect(g.points.at(-1)!.x).toBe(l.width - l.right)
    expect(g.points[1]!.y).toBeCloseTo(l.occTop + l.occHeight * (1 - 13 / 20), 0)
    expect(g.area.endsWith('Z')).toBe(true)
  })

  it('labels local hours and falls back to the first bucket', () => {
    expect(curveGeometry(curve, tz, l).xLabels.map(x => x.label)).toEqual(['00:00'])
    expect(curveGeometry(curve.slice(0, 1), tz, l).xLabels.map(x => x.label)).toEqual(['23:45'])
  })

  it('is empty without buckets and maps x to a clamped bucket', () => {
    const g = curveGeometry([], tz, l)
    expect(g.line).toBe('')
    expect(g.bars).toEqual([])
    expect(bucketAt(0, 3, l)).toBe(0)
    expect(bucketAt(l.width, 3, l)).toBe(2)
    expect(bucketAt(l.left + (l.width - l.left - l.right) / 2, 3, l)).toBe(1)
    expect(bucketAt(10, 0, l)).toBe(-1)
  })
})

describe('barPath', () => {
  it('rounds only the data end and draws nothing for zero', () => {
    expect(barPath(10, 20, 12, 30, 'up')).toBe('M10,50 V24 Q10,20 14,20 H18 Q22,20 22,24 V50 Z')
    expect(barPath(10, 50, 12, 30, 'down')).toBe('M10,50 V76 Q10,80 14,80 H18 Q22,80 22,76 V50 Z')
    expect(barPath(10, 20, 12, 2, 'up')).toContain('Q10,20 12,20')
    expect(barPath(10, 20, 12, 0, 'up')).toBe('')
  })
})
