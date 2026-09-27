/**
 * Pure helpers of the post-event report (P2.4): number formatting, the
 * 15-minute bucket grid in the event's timezone, table ordering and the
 * geometry of the check-in curve (inline SVG, no chart library).
 */
import type { ApiError } from '~/types/event'
import type { CurvePoint, EventReport, ReportListRow, ReportSubmitterRow, ReportTotals } from '~/types/report'
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

/**
 * Everyone who came in through the door: check-ins (list guests and tickets,
 * re-entry counted again) plus manual ins, which the API already folds into
 * each bucket's `in`, plus walk-ups. Walk-ups are the only thing not in `in`.
 */
export function throughTheDoor(curve: CurvePoint[]): number {
  return curve.reduce((s, c) => s + c.in + c.walkups, 0)
}

/** Index of the first bucket reaching the highest occupancy, −1 when empty. */
export function peakIndex(curve: CurvePoint[]): number {
  let best = -1
  curve.forEach((c, i) => {
    if (best < 0 || c.occupancy > curve[best]!.occupancy) best = i
  })
  return best
}

/**
 * "x / y" only while x ≤ y: arrived can exceed going (a pending guest
 * admitted anyway), scanned can exceed valid, so a ratio above 100 % never
 * shows; the tile explains it in words instead.
 */
export function countOf(part: number, whole: number): { of: string | null, over: boolean } {
  return part > whole ? { of: null, over: true } : { of: formatCount(whole), over: false }
}

/** Table cell form of countOf: "6 / 8", or "9 (8 going)" when the part exceeds the whole. */
export function countPair(part: number, whole: number, noun: string): string {
  return part > whole ? `${formatCount(part)} (${formatCount(whole)} ${noun})` : `${formatCount(part)} / ${formatCount(whole)}`
}

export interface KpiTile {
  id: string
  label: string
  value: string
  of: string | null
  sub: string
  /** the headline tile, drawn larger */
  hero?: boolean
}

/**
 * The report's headline tiles. THROUGH THE DOOR comes first because it is
 * the one number that includes walk-ups and manual counts (so it is never
 * below the peak); the other tiles each count one thing and say so.
 */
export function kpiTiles(t: ReportTotals, curve: CurvePoint[], capacity: number | null, tz: string): KpiTile[] {
  const door = throughTheDoor(curve)
  const arrived = countOf(t.guests_arrived, t.guests_going)
  const heads = countOf(t.heads_admitted, t.heads_expected)
  const tickets = countOf(t.tickets_scanned, t.tickets_valid)
  const noShows = t.no_show_rate === null ? 0 : Math.round(t.no_show_rate * t.guests_going)
  const peak = t.peak_at ? bucketRange(t.peak_at, tz) : null
  const going = formatCount(t.guests_going)

  const tiles: KpiTile[] = [
    {
      id: 'door', label: 'THROUGH THE DOOR', value: formatCount(door), of: null, hero: true,
      sub: `${capacity ? `of ${formatCount(capacity)} capacity · ` : ''}peak ${formatCount(t.peak_occupancy)} inside`,
    },
    {
      id: 'arrived', label: 'ARRIVED', value: formatCount(t.guests_arrived), of: arrived.of,
      sub: arrived.over
        ? `${going} going; includes pending or declined guests admitted at the door`
        : `of ${going} going guests showed up`,
    },
    {
      id: 'no-show', label: 'NO-SHOW', value: formatPercent(t.no_show_rate), of: null,
      sub: t.guests_going ? `${formatCount(noShows)} of ${going} going guests didn't come` : 'nobody was on a list as going',
    },
    {
      id: 'heads', label: 'LIST & TICKET HEADS', value: formatCount(t.heads_admitted), of: heads.of,
      sub: heads.over
        ? `checked in, more than the ${formatCount(t.heads_expected)} expected (re-entry counts again)`
        : `checked in of ${formatCount(t.heads_expected)} expected`,
    },
  ]
  if (t.plus_ones_allowed > 0) {
    tiles.push({
      id: 'plus-ones', label: '+1s USED', value: formatCount(t.plus_ones_used), of: formatCount(t.plus_ones_allowed),
      sub: `of ${formatCount(t.plus_ones_allowed)} +1s allowed were used`,
    })
  }
  if (t.tickets_valid > 0 || t.tickets_scanned > 0) {
    tiles.push({
      id: 'tickets', label: 'TICKETS SCANNED', value: formatCount(t.tickets_scanned), of: tickets.of,
      sub: tickets.over
        ? `${formatCount(t.tickets_valid)} valid; includes refunded or void tickets scanned at the door`
        : `of ${formatCount(t.tickets_valid)} valid tickets scanned`,
    })
  }
  tiles.push(
    { id: 'walkups', label: 'WALK-UPS', value: formatCount(t.walkups), of: null, sub: 'paid at the door' },
    { id: 'peak', label: 'PEAK INSIDE', value: formatCount(t.peak_occupancy), of: null, sub: peak ? `busiest 15 min: ${peak}` : 'no one inside yet' },
  )
  return tiles
}

/** "JUST NOW", "1 MIN AGO", "2 H AGO" since the report was generated. */
export function updatedAgo(iso: string, now = Date.now()): string {
  const min = Math.floor((now - Date.parse(iso)) / 60_000)
  if (!Number.isFinite(min) || min < 1) return 'JUST NOW'
  if (min < 60) return `${min} MIN AGO`
  return `${Math.floor(min / 60)} H AGO`
}

/** What a failed report load means for the user, and whether RETRY can help. */
export function loadErrorMessage(err: Pick<ApiError, 'error'>, hasReport: boolean): { text: string, retry: boolean } {
  if (err.error === 'forbidden' || err.error === 'no_role_grant') {
    return { text: 'Your role cannot see this event\'s report. Ask an owner or manager for access.', retry: false }
  }
  if (err.error === 'not_found') return { text: 'This event no longer exists.', retry: false }
  return {
    text: hasReport
      ? 'COULD NOT REFRESH THE REPORT. SHOWING THE LAST NUMBERS. Check your connection.'
      : 'COULD NOT LOAD THE REPORT. Check your connection.',
    retry: true,
  }
}

/** RFC 4180 records (quoted fields may hold commas, quotes and newlines). */
export function parseCsvRecords(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let field = ''
  let quoted = false
  const src = text.replace(/^\uFEFF/, '')
  for (let i = 0; i < src.length; i++) {
    const ch = src[i]!
    if (quoted) {
      if (ch === '"' && src[i + 1] === '"') { field += '"'; i++ }
      else if (ch === '"') quoted = false
      else field += ch
    } else if (ch === '"') quoted = true
    else if (ch === ',') { row.push(field); field = '' }
    else if (ch === '\n' || ch === '\r') {
      if (ch === '\r' && src[i + 1] === '\n') i++
      row.push(field); rows.push(row); row = []; field = ''
    } else field += ch
  }
  if (field || row.length) { row.push(field); rows.push(row) }
  return rows
}

/** Guests and arrivals in a list-back CSV, for the confirmation line. */
export function listBackCounts(csv: string): { guests: number, arrived: number } {
  const [header, ...rows] = parseCsvRecords(csv).filter(r => r.some(f => f !== ''))
  const col = header?.indexOf('arrived') ?? -1
  return { guests: rows.length, arrived: col < 0 ? 0 : rows.filter(r => r[col] === 'yes').length }
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

/**
 * Where the peak's direct label goes: above the dot (pushed down to stay
 * inside the panel), unless that would cover the dot; then beside it (right, or left near the right
 * edge), vertically centred on the dot.
 */
export function peakLabelPos(p: { x: number, y: number }, l: CurveLayout): { x: number, y: number, anchor: 'start' | 'middle' | 'end' } {
  // 11 px text: its baseline needs ≥ 11 px of panel above it and must clear the 4 px dot.
  const above = Math.max(p.y - 9, l.occTop + 11)
  if (above <= p.y - 6) return { x: p.x, y: r1(above), anchor: 'middle' }
  const y = r1(Math.max(l.occTop + 11, p.y + 4))
  return p.x + 40 <= l.width - l.right ? { x: r1(p.x + 8), y, anchor: 'start' } : { x: r1(p.x - 8), y, anchor: 'end' }
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
