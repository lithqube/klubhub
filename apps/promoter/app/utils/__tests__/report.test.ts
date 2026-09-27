import { describe, expect, it } from 'vitest'
import type { CurvePoint, ReportListRow, ReportSubmitterRow, ReportTotals } from '~/types/report'
import {
  barPath, bucketAt, bucketLabel, bucketRange, bucketStart, countOf, countPair, curveGeometry, defaultLayout, formatCount, formatPercent, hasActivity,
  kpiTiles, listBackCounts, listBackFilename, loadErrorMessage, niceMax, parseCsvRecords, peakIndex, peakLabelPos, reportPhase, share, sortLists,
  sortSubmitters, throughTheDoor, updatedAgo,
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

describe('headline tiles', () => {
  const tz = 'Europe/Berlin'
  const pt = (bucket_start: string, i: number, o: number, walkups: number, occupancy: number): CurvePoint => ({ bucket_start, in: i, out: o, walkups, occupancy })
  // `in` already holds check-ins plus manual ins; walk-ups are separate.
  const curve = [
    pt('2026-01-10T23:00:00Z', 5, 0, 2, 7),
    pt('2026-01-10T23:15:00Z', 6, 1, 4, 16),
    pt('2026-01-10T23:30:00Z', 0, 3, 0, 13),
  ]
  const totals: ReportTotals = {
    guests_going: 8, guests_arrived: 6, no_show_rate: 0.25, heads_expected: 16, heads_admitted: 9, plus_ones_allowed: 5, plus_ones_used: 3,
    tickets_valid: 3, tickets_scanned: 2, walkups: 6, peak_occupancy: 16, peak_at: '2026-01-10T23:15:00Z', conflicts: 0,
  }
  const tile = (tiles: ReturnType<typeof kpiTiles>, id: string) => tiles.find(t => t.id === id)

  it('counts everyone through the door once: in (with manual ins) plus walk-ups', () => {
    expect(throughTheDoor(curve)).toBe(17)
    expect(throughTheDoor([])).toBe(0)
    const tiles = kpiTiles(totals, curve, 400, tz)
    expect(tiles[0]).toMatchObject({ id: 'door', value: '17', of: null, hero: true, sub: 'of 400 capacity · peak 16 inside' })
    expect(Number(tiles[0]!.value)).toBeGreaterThanOrEqual(totals.peak_occupancy)
    expect(kpiTiles(totals, curve, null, tz)[0]!.sub).toBe('peak 16 inside')
  })

  it('names list & ticket heads without capacity and writes subs as sentences', () => {
    const tiles = kpiTiles(totals, curve, 400, tz)
    expect(tile(tiles, 'heads')).toMatchObject({ label: 'LIST & TICKET HEADS', value: '9', of: '16', sub: 'checked in of 16 expected' })
    expect(tile(tiles, 'arrived')).toMatchObject({ value: '6', of: '8', sub: 'of 8 going guests showed up' })
    expect(tile(tiles, 'no-show')!.sub).toBe("2 of 8 going guests didn't come")
    expect(tile(tiles, 'plus-ones')!.sub).toBe('of 5 +1s allowed were used')
  })

  it('never shows a ratio above 100 %', () => {
    expect(countOf(3, 4)).toEqual({ of: '4', over: false })
    expect(countOf(4, 4)).toEqual({ of: '4', over: false })
    expect(countOf(5, 4)).toEqual({ of: null, over: true })
    expect(countPair(6, 8, 'going')).toBe('6 / 8')
    expect(countPair(9, 8, 'going')).toBe('9 (8 going)')
    const tiles = kpiTiles({ ...totals, guests_arrived: 9, tickets_scanned: 4, heads_admitted: 20 }, curve, null, tz)
    expect(tile(tiles, 'arrived')).toMatchObject({ of: null, sub: '8 going; includes pending or declined guests admitted at the door' })
    expect(tile(tiles, 'tickets')).toMatchObject({ of: null, sub: '3 valid; includes refunded or void tickets scanned at the door' })
    expect(tile(tiles, 'heads')!.of).toBeNull()
  })

  it('shows the peak as a 15-minute range', () => {
    expect(tile(kpiTiles(totals, curve, null, tz), 'peak')!.sub).toBe('busiest 15 min: 00:15–00:30')
    expect(tile(kpiTiles({ ...totals, peak_at: null, peak_occupancy: 0 }, [], null, tz), 'peak')!.sub).toBe('no one inside yet')
    expect(peakIndex(curve)).toBe(1)
    expect(peakIndex([])).toBe(-1)
  })

  it('hides TICKETS without tickets and +1s when none were allowed', () => {
    const ids = kpiTiles({ ...totals, tickets_valid: 0, tickets_scanned: 0, plus_ones_allowed: 0, plus_ones_used: 0 }, curve, null, tz).map(t => t.id)
    expect(ids).toEqual(['door', 'arrived', 'no-show', 'heads', 'walkups', 'peak'])
    // A scanned ticket of another status still shows the tile.
    expect(kpiTiles({ ...totals, tickets_valid: 0, tickets_scanned: 1 }, curve, null, tz).map(t => t.id)).toContain('tickets')
  })
})

describe('peak label placement', () => {
  const l = defaultLayout(400)
  it('sits above the dot, or beside it when it would clip at the top', () => {
    expect(peakLabelPos({ x: 100, y: 80 }, l)).toEqual({ x: 100, y: 71, anchor: 'middle' })
    // Near the top: pushed down inside the panel while it still clears the dot.
    expect(peakLabelPos({ x: 100, y: l.occTop + 18 }, l)).toEqual({ x: 100, y: l.occTop + 11, anchor: 'middle' })
    const top = peakLabelPos({ x: 100, y: l.occTop }, l)
    expect(top.anchor).toBe('start')
    expect(top.x).toBe(108)
    expect(top.y).toBeGreaterThanOrEqual(l.occTop + 11)
    expect(peakLabelPos({ x: l.width - l.right, y: l.occTop }, l).anchor).toBe('end')
  })
})

describe('status line, errors and list-back', () => {
  const now = Date.parse('2026-01-10T23:10:00Z')
  it('says how long ago the report was generated', () => {
    expect(updatedAgo('2026-01-10T23:09:40Z', now)).toBe('JUST NOW')
    expect(updatedAgo('2026-01-10T23:11:00Z', now)).toBe('JUST NOW')
    expect(updatedAgo('2026-01-10T23:08:00Z', now)).toBe('2 MIN AGO')
    expect(updatedAgo('2026-01-10T21:00:00Z', now)).toBe('2 H AGO')
  })

  it('branches load errors on the code', () => {
    expect(loadErrorMessage({ error: 'forbidden' }, false)).toMatchObject({ retry: false })
    expect(loadErrorMessage({ error: 'no_role_grant' }, true).text).toMatch(/role/)
    expect(loadErrorMessage({ error: 'not_found' }, false)).toEqual({ text: 'This event no longer exists.', retry: false })
    expect(loadErrorMessage({ error: 'network_error' }, false)).toEqual({ text: 'COULD NOT LOAD THE REPORT. Check your connection.', retry: true })
    expect(loadErrorMessage({ error: 'unavailable' }, true).text).toMatch(/^COULD NOT REFRESH/)
  })

  it('counts guests and arrivals in a list-back CSV, quoted fields included', () => {
    const csv = '\uFEFFname,plus_n,status,arrived,heads_admitted,first_in_local\r\n'
      + '"Lorenz, Pia",1,going,yes,2,2026-01-10 23:05\r\n'
      + '"Say ""hi""\nthere",0,pending,no,0,\r\n'
      + 'Ben,0,declined,yes,1,2026-01-11 00:10\r\n'
    expect(parseCsvRecords(csv)[1]![0]).toBe('Lorenz, Pia')
    expect(parseCsvRecords(csv)[2]![0]).toBe('Say "hi"\nthere')
    expect(listBackCounts(csv)).toEqual({ guests: 3, arrived: 2 })
    expect(listBackCounts('name,arrived\n')).toEqual({ guests: 0, arrived: 0 })
  })
})
