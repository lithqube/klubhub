/**
 * Wall-clock ↔ instant conversion in an IANA timezone without a date
 * library. Events are stored as UTC instants; organisers think in local
 * wall time ("23:00 to 07:00 in Berlin"). Overnight ends roll to the next
 * day; DST changes are handled by resolving each wall time on its own.
 */

/** Offset (minutes east of UTC) of tz at instant ms. */
export function tzOffsetMinutes(tz: string, ms: number): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: tz, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  }).formatToParts(new Date(ms))
  const get = (t: string) => Number(parts.find(p => p.type === t)?.value)
  const asUTC = Date.UTC(get('year'), get('month') - 1, get('day'), get('hour'), get('minute'), get('second'))
  return Math.round((asUTC - ms) / 60000)
}

/** Instant for a local date (YYYY-MM-DD) and time (HH:MM) in tz. */
export function zonedToInstant(date: string, time: string, tz: string): Date {
  const [y, mo, d] = date.split('-').map(Number) as [number, number, number]
  const [h, mi] = time.split(':').map(Number) as [number, number]
  const guess = Date.UTC(y, mo - 1, d, h, mi)
  // Two passes settle the offset across a DST boundary.
  let ms = guess - tzOffsetMinutes(tz, guess) * 60000
  ms = guess - tzOffsetMinutes(tz, ms) * 60000
  return new Date(ms)
}

/** Local date (YYYY-MM-DD) and time (HH:MM) of an instant in tz. */
export function instantToZoned(iso: string, tz: string): { date: string, time: string } {
  const f = new Intl.DateTimeFormat('en-CA', { timeZone: tz, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
  const p = Object.fromEntries(f.formatToParts(new Date(iso)).map(x => [x.type, x.value]))
  return { date: `${p.year}-${p.month}-${p.day}`, time: `${p.hour}:${p.minute}` }
}

function addDays(date: string, days: number): string {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}

export interface NightWindow {
  starts_at: string
  ends_at: string
  doors_at: string | null
  /** true when the end falls on the next local day */
  overnight: boolean
  durationMinutes: number
  /** wall-clock length differs from real length (a clock change inside) */
  dstShiftMinutes: number
}

/**
 * Resolve "DATE, doors D, start S, end E" in tz. An end at or before the
 * start is the next day; doors after the start are the previous day.
 */
export function resolveNight(date: string, start: string, end: string, doors: string | null, tz: string): NightWindow {
  const startAt = zonedToInstant(date, start, tz)
  const overnight = end <= start
  const endAt = zonedToInstant(overnight ? addDays(date, 1) : date, end, tz)
  const doorsAt = doors ? zonedToInstant(doors > start ? addDays(date, -1) : date, doors, tz) : null
  const minutes = (s: string) => Number(s.slice(0, 2)) * 60 + Number(s.slice(3, 5))
  const wall = (minutes(end) - minutes(start) + (overnight ? 1440 : 0))
  const real = Math.round((endAt.getTime() - startAt.getTime()) / 60000)
  return {
    starts_at: startAt.toISOString(), ends_at: endAt.toISOString(), doors_at: doorsAt?.toISOString() ?? null,
    overnight, durationMinutes: real, dstShiftMinutes: real - wall,
  }
}

/** "SAT 03 OCT" in tz. */
export function dayLabel(iso: string, tz: string): string {
  return new Intl.DateTimeFormat('en-GB', { timeZone: tz, weekday: 'short', day: '2-digit', month: 'short' })
    .format(new Date(iso)).replace(',', '').toUpperCase()
}

/** "23:00" in tz. */
export function timeLabel(iso: string, tz: string): string {
  return instantToZoned(iso, tz).time
}

/** "23:00 → 07:00 +1" in tz. */
export function rangeLabel(startIso: string, endIso: string, tz: string): string {
  const s = instantToZoned(startIso, tz)
  const e = instantToZoned(endIso, tz)
  const plus = e.date > s.date ? ' +1' : ''
  return `${s.time} → ${e.time}${plus}`
}

/** Short zone name, e.g. "CEST". */
export function zoneAbbrev(iso: string, tz: string): string {
  return new Intl.DateTimeFormat('en-GB', { timeZone: tz, timeZoneName: 'short' })
    .formatToParts(new Date(iso)).find(p => p.type === 'timeZoneName')?.value ?? tz
}

export function durationLabel(minutes: number): string {
  const h = Math.floor(minutes / 60)
  const m = minutes % 60
  return m ? `${h}H ${m}′` : `${h}H`
}
