/**
 * Pure helpers of the post-event report (P2.4): number formatting, the
 * 15-minute bucket grid in the event's timezone, table ordering and the
 * geometry of the check-in curve (inline SVG, no chart library).
 */
import type { CurvePoint, EventReport, ReportListRow, ReportSubmitterRow } from '~/types/report'
import { timeLabel, tzOffsetMinutes } from '~/utils/datetime'

export const BUCKET_MS = 15 * 60_000

/** "38%" from a 0–1 rate; null (nobody going) reads "—". */
export function formatPercent(rate: number | null | undefined): string {
  if (rate === null || rate === undefined || !Number.isFinite(rate)) return '—'
  return `${Math.round(rate * 100)}%`
}

/** Share of a whole as a rate, null when the whole is 0. */
export function share(part: number, whole: number | null | undefined): number | null {
  return whole ? part / whole : null
}

/** Grouped integer: 1,284. */
export function formatCount(n: number): string {
  return new Intl.NumberFormat('en-US').format(n)
}

/**
 * Start of the 15-minute bucket holding instant ms, aligned to
 * :00/:15/:30/:45 of the wall clock in tz (so a +05:45 zone still gets
 * local quarter hours).
 */
export function bucketStart(ms: number, tz: string): number {
  const off = tzOffsetMinutes(tz, ms) * 60_000
  return Math.floor((ms + off) / BUCKET_MS) * BUCKET_MS - off
}

/** "23:15" — a bucket's start in the event's timezone. */
export function bucketLabel(iso: string, tz: string): string {
  return timeLabel(iso, tz)
}

/** "23:15–23:30" — a bucket's span in the event's timezone. */
export function bucketRange(iso: string, tz: string): string {
  return `${timeLabel(iso, tz)}–${timeLabel(new Date(Date.parse(iso) + BUCKET_MS).toISOString(), tz)}`
}

export type ReportPhase = 'upcoming' | 'live' | 'past'

/** LIVE only once the night has started; `live` alone is also true before it. */
export function reportPhase(r: Pick<EventReport, 'live' | 'event'>, now = Date.now()): ReportPhase {
  if (!r.live) return 'past'
  return Date.parse(r.event.starts_at) <= now ? 'live' : 'upcoming'
}

/** Anything happened at the door (check-ins, walk-ups, manual counts). */
export function hasActivity(r: Pick<EventReport, 'curve' | 'totals'>): boolean {
  return r.curve.length > 0 || r.totals.heads_admitted > 0 || r.totals.walkups > 0 || r.totals.tickets_scanned > 0
}

const artistFirst = (a: string, b: string) => Number(b === 'artist') - Number(a === 'artist')

/** Lists in report order: artist lists first, the rest as the API sent them. */
export function sortLists(rows: ReportListRow[]): ReportListRow[] {
  return rows.map((r, i) => ({ r, i })).sort((a, b) => artistFirst(a.r.type, b.r.type) || a.i - b.i).map(x => x.r)
}

/**
 * Allocations in report order: artist lists first, the rest as the API sent
 * them (list position, then allocation creation; revoked ones included).
 */
export function sortSubmitters(rows: ReportSubmitterRow[]): ReportSubmitterRow[] {
  return rows.map((r, i) => ({ r, i })).sort((a, b) => artistFirst(a.r.list_type, b.r.list_type) || a.i - b.i).map(x => x.r)
}

/** A download filename part: lowercase ASCII words joined by dashes. */
export function slugPart(s: string): string {
  return s.normalize('NFKD').replace(/[̀-ͯ]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40) || 'list'
}

/** Same name the API sends in Content-Disposition: <slug>-list-back-<label>.csv. */
export function listBackFilename(eventSlug: string, submitter: string): string {
  return `${eventSlug}-list-back-${slugPart(submitter)}.csv`
}

/** Round a head count up to 1, 2 or 5 × 10^k (at least 1) so axis ticks are clean. */
export function niceMax(n: number): number {
  if (!(n > 0)) return 1
  const p = 10 ** Math.floor(Math.log10(n))
  const m = n / p
  return Math.max(1, (m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10) * p)
}

// ---------------------------------------------------------------- curve geometry

export interface CurveLayout {
  width: number
  /** left gutter for y tick labels */
  left: number
  right: number
  occTop: number
  occHeight: number
  flowTop: number
  flowHeight: number
  /** bottom band for x labels */
  axisHeight: number
}

export const defaultLayout = (width: number): CurveLayout => ({
  width: Math.max(240, Math.round(width)), left: 34, right: 8, occTop: 8, occHeight: 120, flowTop: 150, flowHeight: 90, axisHeight: 22,
})

export const layoutHeight = (l: CurveLayout) => l.flowTop + l.flowHeight + l.axisHeight

export interface CurveBar {
  i: number
  /** band start and width (the hover / focus target) */
  bandX: number
  bandW: number
  /** painted bar */
  x: number
  w: number
  upY: number
  upH: number
  downY: number
  downH: number
}

export interface CurveGeometry {
  bars: CurveBar[]
  /** occupancy line (value at each bucket's end) and its area wash */
  line: string
  area: string
  /** end-of-bucket occupancy points */
  points: { x: number, y: number }[]
  occMax: number
  occTicks: { y: number, value: number }[]
  upMax: number
  downMax: number
  baseline: number
  flowTicks: { y: number, value: number }[]
  xLabels: { x: number, label: string }[]
}

const r1 = (n: number) => Math.round(n * 10) / 10

/**
 * Two panels sharing one time axis: occupancy (line + wash) on top; heads
 * per 15 minutes below, arrivals (check-ins + walk-ups) above the baseline
 * and exits below it. Each panel has one y-scale.
 */
export function curveGeometry(curve: CurvePoint[], tz: string, l: CurveLayout): CurveGeometry {
  const n = curve.length
  const plotW = l.width - l.left - l.right
  const bandW = n ? plotW / n : plotW
  const barW = Math.max(1, Math.min(24, bandW - 2))

  const occMax = niceMax(Math.max(0, ...curve.map(c => c.occupancy)))
  const upMax = niceMax(Math.max(0, ...curve.map(c => c.in + c.walkups)))
  const rawDown = Math.max(0, ...curve.map(c => c.out))
  const downMax = rawDown ? niceMax(rawDown) : 0
  const flowScale = l.flowHeight / (upMax + downMax)
  const baseline = l.flowTop + upMax * flowScale
  const occY = (v: number) => l.occTop + l.occHeight - (v / occMax) * l.occHeight

  const bars: CurveBar[] = curve.map((c, i) => {
    const bandX = l.left + i * bandW
    const upH = (c.in + c.walkups) * flowScale
    const downH = c.out * flowScale
    return {
      i, bandX: r1(bandX), bandW: r1(bandW), x: r1(bandX + (bandW - barW) / 2), w: r1(barW),
      upY: r1(baseline - upH), upH: r1(upH), downY: r1(baseline), downH: r1(downH),
    }
  })

  const points = curve.map((c, i) => ({ x: r1(l.left + (i + 1) * bandW), y: r1(occY(c.occupancy)) }))
  const floor = r1(occY(0))
  const start = `M${l.left},${floor}`
  const line = n ? `${start} ${points.map(p => `L${p.x},${p.y}`).join(' ')}` : ''
  const area = n ? `${line} L${points.at(-1)!.x},${floor} Z` : ''

  const occTicks = (occMax >= 2 ? [0, occMax / 2, occMax] : [0, occMax]).map(v => ({ y: r1(occY(v)), value: v }))
  const flowTicks = [{ y: r1(l.flowTop), value: upMax }, { y: r1(baseline), value: 0 }]
  if (downMax) flowTicks.push({ y: r1(l.flowTop + l.flowHeight), value: -downMax })

  // Hour labels at bucket starts on the hour, thinned to stay ≥ 48 px apart.
  const hours = curve.map((c, i) => ({ i, label: bucketLabel(c.bucket_start, tz) })).filter(h => h.label.endsWith(':00'))
  const everyHours = Math.max(1, Math.ceil(48 / (bandW * 4)))
  let xLabels = hours.filter((_, k) => k % everyHours === 0).map(h => ({ x: r1(l.left + h.i * bandW), label: h.label }))
  if (!xLabels.length && n) xLabels = [{ x: l.left, label: bucketLabel(curve[0]!.bucket_start, tz) }]

  return { bars, line, area, points, occMax, occTicks, upMax, downMax, baseline: r1(baseline), flowTicks, xLabels }
}

/** Index of the bucket under x (clamped), for the crosshair. */
export function bucketAt(x: number, count: number, l: CurveLayout): number {
  if (!count) return -1
  const bandW = (l.width - l.left - l.right) / count
  return Math.min(count - 1, Math.max(0, Math.floor((x - l.left) / bandW)))
}

/**
 * A bar growing from the baseline: square at the baseline, 4 px rounded
 * data end (up: top corners, down: bottom corners). Empty when h is 0.
 */
export function barPath(x: number, y: number, w: number, h: number, dir: 'up' | 'down'): string {
  if (!(h > 0) || !(w > 0)) return ''
  const r = r1(Math.min(4, w / 2, h))
  const x2 = r1(x + w)
  const y2 = r1(y + h)
  return dir === 'up'
    ? `M${x},${y2} V${r1(y + r)} Q${x},${y} ${r1(x + r)},${y} H${r1(x2 - r)} Q${x2},${y} ${x2},${r1(y + r)} V${y2} Z`
    : `M${x},${y} V${r1(y2 - r)} Q${x},${y2} ${r1(x + r)},${y2} H${r1(x2 - r)} Q${x2},${y2} ${x2},${r1(y2 - r)} V${y} Z`
}
