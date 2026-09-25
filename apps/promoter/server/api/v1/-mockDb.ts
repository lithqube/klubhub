// In-memory data for frontend-only development (no NUXT_PUBLIC_API_BASE).
// Shapes mirror the Go API; timetable issues come from the shared client
// validator so the UI behaves as it will against the server.
import type { EventDetail, EventSummary, Venue } from '~/types/event'
import { validateTimetable } from '~/utils/timetable'

const DAY = 86_400_000
const base = new Date()
base.setUTCHours(21, 0, 0, 0) // 23:00 Berlin (CEST)
const iso = (offsetDays: number, hours = 0, minutes = 0) =>
  new Date(base.getTime() + offsetDays * DAY + (hours * 60 + minutes) * 60_000).toISOString()

export const venues: Venue[] = [{
  id: 'v-tresor', name: 'Tresor.West', city: 'Berlin', country: 'DE', timezone: 'Europe/Berlin', capacity: 1200,
  curfew_local: '05:00', tech_notes: '2× CDJ-3000, DJM-V10, Funktion-One', archived_at: null,
  protected: { address: true, geo: false, contact: true },
  rooms: [{ id: 'r-main', name: 'Main Room', capacity: 800, position: 0 }, { id: 'r-garden', name: 'Garden', capacity: 400, position: 1 }],
}]

function detail(e: Omit<EventDetail, 'issues'>): EventDetail {
  return { ...e, issues: validateTimetable(e, e.stages, e.lineup) }
}

const common = {
  timezone: 'Europe/Berlin', visibility: 'public' as const, publish_at: null, location_reveal_at: null, min_age: 18,
  genres: ['techno'], description_md: 'Eight hours across two rooms.', cost_text: '€20 / €25 at the door',
  capacity: 1200, created_at: iso(-10), updated_at: iso(-1),
}

export const events: EventDetail[] = [
  detail({
    ...common, id: 'e-klubnacht', title: 'Klubnacht 03', slug: 'klubnacht-03', status: 'published',
    starts_at: iso(7), ends_at: iso(7, 8), doors_at: iso(7, -0.5), venue_id: 'v-tresor', city: 'Berlin',
    location_mode: 'venue', external_ticket_url: 'https://tickets.example/klubnacht-03', version: 4,
    venue: { id: 'v-tresor', name: 'Tresor.West', city: 'Berlin' },
    stages: [
      { id: 's-main', name: 'Main Room', position: 0, curfew_at: iso(7, 6), changeover_minutes: 15 },
      { id: 's-garden', name: 'Garden', position: 1, curfew_at: null, changeover_minutes: 10 },
    ],
    lineup: [
      { id: 'l1', stage_id: 's-main', display_name: 'Ben Klock', profile_url: 'https://ra.co/dj/benklock', billing_order: 0, b2b_group: null, set_start: iso(7, 3, 15), set_end: iso(7, 6) },
      { id: 'l2', stage_id: 's-main', display_name: 'Dasha Rush', profile_url: null, billing_order: 1, b2b_group: null, set_start: iso(7, 1), set_end: iso(7, 3) },
      { id: 'l3', stage_id: 's-main', display_name: 'Kaiser', profile_url: null, billing_order: 3, b2b_group: null, set_start: iso(7, 0), set_end: iso(7, 0, 45) },
      { id: 'l4', stage_id: null, display_name: 'Oscar Mulero', profile_url: null, billing_order: 2, b2b_group: null, set_start: null, set_end: null },
      { id: 'l5', stage_id: 's-garden', display_name: 'Lena W', profile_url: null, billing_order: 4, b2b_group: null, set_start: iso(7, 0), set_end: iso(7, 2, 30) },
    ],
  }),
  detail({
    ...common, id: 'e-warehouse', title: 'Warehouse 10', slug: 'warehouse-10', status: 'scheduled', publish_at: iso(2),
    starts_at: iso(14), ends_at: iso(14, 10), doors_at: null, venue_id: 'v-tresor', city: 'Berlin',
    location_mode: 'secret', location_reveal_at: iso(13, -9), external_ticket_url: null, version: 2,
    venue: { id: 'v-tresor', name: 'Tresor.West', city: 'Berlin' }, stages: [], lineup: [],
  }),
  detail({
    ...common, id: 'e-halloween', title: 'Halloween Special', slug: 'halloween-special', status: 'draft',
    starts_at: iso(20), ends_at: iso(20, 7), doors_at: null, venue_id: null, city: 'Berlin', location_mode: 'city_only',
    external_ticket_url: null, version: 1, venue: null, stages: [], lineup: [], min_age: null, genres: [], cost_text: '',
  }),
]

export function summary(e: EventDetail): EventSummary {
  const { venue, stages, lineup, issues: _issues, ...rest } = e
  return {
    ...rest, venue_name: venue?.name ?? null, act_count: lineup.length, stage_count: stages.length,
    untimed_count: lineup.filter(l => !l.set_start).length,
  }
}

export function findEvent(id: string): EventDetail {
  const e = events.find(x => x.id === id)
  if (!e) throw createError({ statusCode: 404, data: { error: 'not_found' } })
  return e
}

export function recheck(e: EventDetail): EventDetail {
  e.issues = validateTimetable(e, e.stages, e.lineup)
  e.version += 1
  e.updated_at = new Date().toISOString()
  return e
}

export function assertVersion(e: EventDetail, version: number) {
  if (e.version !== version) throw createError({ statusCode: 409, data: { error: 'version_conflict' } })
}

export const newId = (p: string) => `${p}-${Math.random().toString(36).slice(2, 10)}`
