// In-memory data for frontend-only development (no NUXT_PUBLIC_API_BASE).
// Shapes mirror the Go API; timetable issues come from the shared client
// validator so the UI behaves as it will against the server.
import { pbkdf2Sync, randomBytes, randomInt } from 'node:crypto'
import type {
  AddResult as DoorAddResult, DoorAdd, DoorBundle, DoorCheckin, DoorCounters, DoorDevice, DoorOp, DoorSubject, ManagerPinVerifier, OpResult, PinStatus,
  RegisteredDevice, SyncResponse,
} from '~/types/door'
import type { EventDetail, EventSummary, Venue } from '~/types/event'
import type { CurvePoint, EventReport, ReportSubmitterRow } from '~/types/report'
import type {
  AddGuestsInput, AddResult, Allocation, AllocationInput, BulkResult, BulkStatusInput, EntryTerms, Guest, GuestInput, GuestList, GuestPage,
  GuestStatus, ImportField, ImportPreset, ImportResult, ListInput, OverviewRow, StandingList, Ticket, TicketStatus,
} from '~/types/guest'
import type { Organization, OrgProfile } from '~/types/org'
import type {
  Contact as AudienceContact, ContactInput as AudienceContactInput, ContactSource as AudienceContactSource,
  ContactStatus as AudienceContactStatus, ConsentBasis as AudienceConsentBasis, ConsentInput as AudienceConsentInput,
  Counts as AudienceCounts, ImportResult as AudienceImportResult, Page as AudiencePage, Segment as AudienceSegment,
  SegmentFilter as AudienceSegmentFilter,
} from '~/types/audience'
import {
  allocationState, arrivalsFrom, countByStatus, cutoffInstant, fold, headsHeld, holdsQuota, LIST_TYPES, STATUSES,
} from '~/utils/guests'
import { validateTimetable } from '~/utils/timetable'
import { BUCKET_MS, bucketStart } from '~/utils/report'
import { instantToZoned } from '~/utils/datetime'
import { confirmMatches } from '~/utils/privacy'
import { type CsvTable, type ImportProblem, type ImportRow, mapRow, maskEmail, maskName, resolveMapping } from '~/utils/attendeeImport'

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
    starts_at: iso(6), ends_at: iso(6, 8), doors_at: iso(6, -0.5), venue_id: 'v-tresor', city: 'Berlin',
    location_mode: 'venue', external_ticket_url: 'https://tickets.example/klubnacht-03', version: 4,
    venue: { id: 'v-tresor', name: 'Tresor.West', city: 'Berlin' },
    stages: [
      { id: 's-main', name: 'Main Room', position: 0, curfew_at: iso(6, 6), changeover_minutes: 15 },
      { id: 's-garden', name: 'Garden', position: 1, curfew_at: null, changeover_minutes: 10 },
    ],
    lineup: [
      { id: 'l1', stage_id: 's-main', display_name: 'Ben Klock', profile_url: 'https://ra.co/dj/benklock', billing_order: 0, b2b_group: null, set_start: iso(6, 3, 15), set_end: iso(6, 6) },
      { id: 'l2', stage_id: 's-main', display_name: 'Dasha Rush', profile_url: null, billing_order: 1, b2b_group: null, set_start: iso(6, 1), set_end: iso(6, 3) },
      { id: 'l3', stage_id: 's-main', display_name: 'Kaiser', profile_url: null, billing_order: 3, b2b_group: null, set_start: iso(6, 0), set_end: iso(6, 0, 45) },
      { id: 'l4', stage_id: null, display_name: 'Oscar Mulero', profile_url: null, billing_order: 2, b2b_group: null, set_start: null, set_end: null },
      { id: 'l5', stage_id: 's-garden', display_name: 'Lena W', profile_url: null, billing_order: 4, b2b_group: null, set_start: iso(6, 0), set_end: iso(6, 2, 30) },
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
  // Last week's night: the post-event report (P2.4) has door activity to show.
  detail({
    ...common, id: 'e-klubnacht-02', title: 'Klubnacht 02', slug: 'klubnacht-02', status: 'published',
    starts_at: iso(-7), ends_at: iso(-7, 8), doors_at: null, venue_id: 'v-tresor', city: 'Berlin',
    location_mode: 'venue', external_ticket_url: null, version: 3, capacity: 400,
    venue: { id: 'v-tresor', name: 'Tresor.West', city: 'Berlin' }, stages: [], lineup: [],
  }),
  // Five weeks ago: guest data already erased by retention (P2.5); the rows
  // stay anonymised, so the report and the guest table still have numbers.
  detail({
    ...common, id: 'e-klubnacht-01', title: 'Klubnacht 01', slug: 'klubnacht-01', status: 'published',
    starts_at: iso(-35), ends_at: iso(-35, 8), doors_at: null, venue_id: 'v-tresor', city: 'Berlin',
    location_mode: 'venue', external_ticket_url: null, version: 2, capacity: 400,
    venue: { id: 'v-tresor', name: 'Tresor.West', city: 'Berlin' }, stages: [], lineup: [],
  }),
]

export function summary(e: EventDetail): EventSummary {
  const { venue, stages, lineup, issues, ...rest } = e
  const past = Date.parse(e.ends_at) <= Date.now() && e.status !== 'draft'
  return {
    ...rest, venue_name: venue?.name ?? null, act_count: lineup.length, stage_count: stages.length,
    untimed_count: lineup.filter(l => !l.set_start).length,
    error_count: past ? 0 : issues.filter(i => i.severity === 'error').length,
    warning_count: past ? 0 : issues.filter(i => i.severity === 'warning').length,
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

/** Mock session: an owner who turns on TOTP via ACCOUNT → SECURITY; authAt feeds the ERASE NOW step-up check. */
export const mockSession = { mfa: false, authAt: Date.now() }

/**
 * The sign-in time of this browser: the kh_mock_auth_at cookie (set by the
 * mock login; e2e can set an old one), else the last mock login anywhere.
 */
export const MOCK_AUTH_COOKIE = 'kh_mock_auth_at'
export function mockAuthAt(event: Parameters<typeof getCookie>[0]): number {
  const n = Number(getCookie(event, MOCK_AUTH_COOKIE))
  return Number.isFinite(n) && n > 0 ? n : mockSession.authAt
}

/**
 * A second factor for this browser: TOTP confirmed (for everyone, as
 * before), or a mock sign-in that carried a code, which sets the
 * kh_mock_mfa cookie (so parallel e2e specs that expect "no 2FA" are not
 * affected).
 */
export const MOCK_MFA_COOKIE = 'kh_mock_mfa'
export function mockMfa(event: Parameters<typeof getCookie>[0]): boolean {
  return mockSession.mfa || getCookie(event, MOCK_MFA_COOKIE) === '1'
}

/** Mock collective profile (P1.5). */
export const orgProfile: OrgProfile = {
  bio: 'Berlin techno nights since 2019.', website_url: 'https://nachtwerk.example', instagram_url: 'https://instagram.com/nachtwerk',
  soundcloud_url: null, ra_url: null, accent_color: null,
}

export const mockOrg = (): Organization => ({
  id: '0190f1d2-7c1a-7a00-9f00-000000000001', name: 'Nachtwerk Collective', slug: 'nachtwerk',
  timezone: 'Europe/Berlin', currency: 'EUR', ...orgProfile,
})

// ---------------------------------------------------------------- guests (P2.1)
// Mirrors api/internal/promoter/guest: quota counts heads (guest + N) of
// going / pending / invited guests; name-only lists refuse contacts; the
// same email in an event or the same name on a list is a duplicate.
// All names below are fictional.

export const standingLists: StandingList[] = [
  { id: 'sl-residents', name: 'Residents', type: 'artist', collect_contact: false, position: 0, entry_terms: { price_mode: 'free', reduced_price_text: '', cutoff_local: '01:00', perks: ['drink token'] } },
  { id: 'sl-crew', name: 'Crew', type: 'crew', collect_contact: true, position: 1, entry_terms: { price_mode: 'free', reduced_price_text: '', perks: [] } },
]

type ListRow = Omit<GuestList, 'allocations' | 'guests' | 'heads' | 'pending' | 'quota'>
type AllocRow = Omit<Allocation, 'used' | 'guests' | 'pending'> & { event_id: string }
type GuestRow = Omit<Guest, 'heads_in' | 'first_in_at'> & { event_id: string }

const terms = (t: Partial<EntryTerms> = {}): EntryTerms => ({ price_mode: 'free', reduced_price_text: '', perks: [], ...t })

export const guestLists: ListRow[] = [
  { id: 'gl-artist', event_id: 'e-klubnacht', name: 'Artist guests', type: 'artist', collect_contact: false, standing_template_id: 'sl-residents', position: 0, entry_terms: terms({ cutoff_at: iso(6, 2), perks: ['drink token'] }) },
  { id: 'gl-comp', event_id: 'e-klubnacht', name: 'Comp', type: 'comp', collect_contact: false, standing_template_id: null, position: 1, entry_terms: terms() },
  { id: 'gl-industry', event_id: 'e-klubnacht', name: 'Industry', type: 'industry', collect_contact: true, standing_template_id: null, position: 2, entry_terms: terms({ price_mode: 'reduced', reduced_price_text: '€10 before 01:00', cutoff_at: iso(6, 2) }) },
  { id: 'gl-wh-comp', event_id: 'e-warehouse', name: 'Comp', type: 'comp', collect_contact: false, standing_template_id: null, position: 0, entry_terms: terms() },
  { id: 'gl-02-comp', event_id: 'e-klubnacht-02', name: 'Comp', type: 'comp', collect_contact: false, standing_template_id: null, position: 0, entry_terms: terms() },
  { id: 'gl-02-artist', event_id: 'e-klubnacht-02', name: 'Artist guests', type: 'artist', collect_contact: false, standing_template_id: 'sl-residents', position: 1, entry_terms: terms({ perks: ['drink token'] }) },
  { id: 'gl-02-promo', event_id: 'e-klubnacht-02', name: 'Promoters', type: 'promoter', collect_contact: false, standing_template_id: null, position: 2, entry_terms: terms() },
  { id: 'gl-01-artist', event_id: 'e-klubnacht-01', name: 'Artist guests', type: 'artist', collect_contact: false, standing_template_id: 'sl-residents', position: 0, entry_terms: terms() },
  { id: 'gl-01-industry', event_id: 'e-klubnacht-01', name: 'Industry', type: 'industry', collect_contact: true, standing_template_id: null, position: 1, entry_terms: terms() },
]

export const allocations: AllocRow[] = [
  { id: 'al-ben', event_id: 'e-klubnacht', list_id: 'gl-artist', label: 'Ben Klock', submitter_contact: 'tour@agency.example', quota: 6, plus_n_max: 1, deadline: iso(6, -3), requires_approval: false, revoked_at: null },
  { id: 'al-dasha', event_id: 'e-klubnacht', list_id: 'gl-artist', label: 'Dasha Rush', submitter_contact: '', quota: 4, plus_n_max: 1, deadline: null, requires_approval: true, revoked_at: null },
  { id: 'al-02-kaiser', event_id: 'e-klubnacht-02', list_id: 'gl-02-artist', label: 'Kaiser', submitter_contact: '', quota: 6, plus_n_max: 1, deadline: null, requires_approval: false, revoked_at: null },
  { id: 'al-02-lena', event_id: 'e-klubnacht-02', list_id: 'gl-02-artist', label: 'Lena W', submitter_contact: '', quota: 4, plus_n_max: 2, deadline: null, requires_approval: false, revoked_at: null },
  { id: 'al-02-crew', event_id: 'e-klubnacht-02', list_id: 'gl-02-promo', label: 'Nachtwerk street team', submitter_contact: '', quota: 4, plus_n_max: 0, deadline: null, requires_approval: false, revoked_at: iso(-8) },
  { id: 'al-01-kaiser', event_id: 'e-klubnacht-01', list_id: 'gl-01-artist', label: 'Kaiser', submitter_contact: 'kaiser@agency.example', quota: 6, plus_n_max: 1, deadline: null, requires_approval: false, revoked_at: null },
]

const seedGuest = (id: string, event_id: string, list_id: string, allocation_id: string | null, name: string, extra: Partial<Guest> = {}): GuestRow => ({
  id, event_id, list_id, allocation_id, name, email: '', phone: '', note: '', plus_n: 0, status: 'going', source: 'manual',
  created_at: iso(-3), updated_at: iso(-3), ...extra,
})

export const guests: GuestRow[] = [
  seedGuest('g1', 'e-klubnacht', 'gl-artist', 'al-ben', 'Mara Weiss', { plus_n: 1 }),
  seedGuest('g2', 'e-klubnacht', 'gl-artist', 'al-ben', 'Tomasz Nowak'),
  seedGuest('g3', 'e-klubnacht', 'gl-artist', 'al-dasha', 'Ines Duarte', { plus_n: 1, status: 'pending', source: 'paste' }),
  seedGuest('g4', 'e-klubnacht', 'gl-artist', 'al-dasha', 'Kofi Mensah', { status: 'pending', source: 'paste' }),
  seedGuest('g5', 'e-klubnacht', 'gl-comp', null, 'Lena Vogt', { status: 'waitlist' }),
  seedGuest('g6', 'e-klubnacht', 'gl-comp', null, 'Sam Oduya', { status: 'invited', note: 'plus-one TBC' }),
  seedGuest('g7', 'e-klubnacht', 'gl-comp', null, 'Juno Park', { status: 'declined' }),
  seedGuest('g8', 'e-klubnacht', 'gl-industry', null, 'Aiko Tanaka', { email: 'aiko@label.example' }),
  seedGuest('g9', 'e-klubnacht', 'gl-industry', null, 'Rafael Ortiz', { email: 'rafael@press.example', status: 'invited' }),
  seedGuest('g10', 'e-warehouse', 'gl-wh-comp', null, 'Noor Haddad', { plus_n: 1 }),
  seedGuest('g11', 'e-warehouse', 'gl-wh-comp', null, 'Emil Sørensen'),
  seedGuest('g02-1', 'e-klubnacht-02', 'gl-02-artist', 'al-02-kaiser', 'Pia Lorenz', { plus_n: 1 }),
  seedGuest('g02-2', 'e-klubnacht-02', 'gl-02-artist', 'al-02-kaiser', 'Otto Brandl'),
  seedGuest('g02-3', 'e-klubnacht-02', 'gl-02-artist', 'al-02-kaiser', 'Yusuf Demir', { plus_n: 1 }),
  seedGuest('g02-4', 'e-klubnacht-02', 'gl-02-artist', 'al-02-lena', 'Carla Mendes', { plus_n: 2 }),
  seedGuest('g02-5', 'e-klubnacht-02', 'gl-02-artist', 'al-02-lena', 'Finn Olsen'),
  seedGuest('g02-6', 'e-klubnacht-02', 'gl-02-comp', null, 'Greta Holm', { plus_n: 1 }),
  seedGuest('g02-7', 'e-klubnacht-02', 'gl-02-comp', null, 'Ivo Petrov', { status: 'declined' }),
  seedGuest('g02-8', 'e-klubnacht-02', 'gl-02-comp', null, 'Ravi Nair'),
  seedGuest('g02-9', 'e-klubnacht-02', 'gl-02-promo', 'al-02-crew', 'Mei Chen'),
  // Erased at start-up (see seedPurged): only list, +N, status and check-ins remain.
  seedGuest('g01-1', 'e-klubnacht-01', 'gl-01-artist', 'al-01-kaiser', 'Anna Roth', { plus_n: 1 }),
  seedGuest('g01-2', 'e-klubnacht-01', 'gl-01-artist', 'al-01-kaiser', 'Ben Adler'),
  seedGuest('g01-3', 'e-klubnacht-01', 'gl-01-industry', null, 'Clara Stein', { email: 'clara@label.example', note: 'label A&R' }),
  seedGuest('g01-4', 'e-klubnacht-01', 'gl-01-industry', null, 'Dario Conti', { email: 'dario@press.example', status: 'declined' }),
]

const guestErr = (statusCode: number, data: Record<string, unknown>) => createError({ statusCode, data })
const invalidField = (field: string, problem: string) => guestErr(422, { error: 'invalid', field, problem })

function strip<T extends { event_id: string }>(row: T): Omit<T, 'event_id'> {
  const { event_id: _e, ...rest } = row
  return rest
}

function allocView(a: AllocRow): Allocation {
  const mine = guests.filter(g => g.allocation_id === a.id)
  return { ...strip(a), used: headsHeld(mine), guests: mine.filter(g => g.status !== 'declined').length, pending: mine.filter(g => g.status === 'pending').length }
}

export function listViews(eventId: string): GuestList[] {
  findEvent(eventId)
  return guestLists.filter(l => l.event_id === eventId).sort((a, b) => a.position - b.position).map((l) => {
    const mine = guests.filter(g => g.list_id === l.id && g.status !== 'declined')
    const allocs = allocations.filter(a => a.list_id === l.id).map(allocView)
      .sort((a, b) => Number(!!a.revoked_at) - Number(!!b.revoked_at))
    return {
      ...l, allocations: allocs, guests: mine.length, heads: mine.reduce((n, g) => n + 1 + g.plus_n, 0),
      pending: mine.filter(g => g.status === 'pending').length,
      quota: allocs.filter(a => !a.revoked_at).reduce((n, a) => n + a.quota, 0),
    }
  })
}

export const listView = (eventId: string, listId: string) => listViews(eventId).find(l => l.id === listId)!

export function findList(eventId: string, listId: string): ListRow {
  const l = guestLists.find(x => x.id === listId && x.event_id === eventId)
  if (!l) throw guestErr(404, { error: 'not_found' })
  return l
}

export function findAllocation(listId: string, id: string): AllocRow {
  const a = allocations.find(x => x.id === id && x.list_id === listId)
  if (!a) throw guestErr(404, { error: 'not_found' })
  return a
}

export function allocationView(a: AllocRow): Allocation {
  return allocView(a)
}

export function validateListInput(b: ListInput, template: boolean): ListInput {
  const name = b?.name?.trim() ?? ''
  if (!name || name.length > 80) throw invalidField('name', 'required, at most 80 characters')
  if (!LIST_TYPES.some(t => t.id === b.type)) throw invalidField('type', LIST_TYPES.map(t => t.id).join(', '))
  const t = { ...terms(), ...b.entry_terms }
  if (t.price_mode === 'reduced' && !t.reduced_price_text?.trim()) throw invalidField('entry_terms.reduced_price_text', 'say what they pay, e.g. €10 before 01:00')
  if (template && t.cutoff_at) throw invalidField('entry_terms.cutoff_at', 'standing lists use a local cutoff time (cutoff_local)')
  if (!template && t.cutoff_local) throw invalidField('entry_terms.cutoff_local', 'event lists use an absolute cutoff (cutoff_at)')
  const perks = [...new Set((t.perks ?? []).map(p => p.trim().toLowerCase()).filter(Boolean))]
  if (perks.length > 8) throw invalidField('entry_terms.perks', 'at most 8')
  return {
    name, type: b.type, collect_contact: !!b.collect_contact,
    entry_terms: { ...t, reduced_price_text: t.price_mode === 'reduced' ? t.reduced_price_text.trim() : '', perks },
  }
}

export function validateAllocationInput(b: AllocationInput): AllocationInput {
  const label = b?.label?.trim() ?? ''
  if (!label || label.length > 120) throw invalidField('label', 'required, at most 120 characters')
  if (!(b.quota >= 1 && b.quota <= 1000)) throw invalidField('quota', '1 to 1000')
  if (!(b.plus_n_max >= 0 && b.plus_n_max <= 10)) throw invalidField('plus_n_max', '0 to 10')
  return { ...b, label }
}

/** Copy standing lists into a new mock event (what the Go event hook does). */
export function copyStandingLists(e: EventDetail) {
  standingLists.forEach((s, i) => {
    const { cutoff_local, ...rest } = s.entry_terms
    guestLists.push({
      id: newId('gl'), event_id: e.id, name: s.name, type: s.type, collect_contact: s.collect_contact, standing_template_id: s.id, position: i,
      entry_terms: { ...rest, cutoff_at: cutoff_local ? cutoffInstant(e.starts_at, e.timezone, cutoff_local) : null },
    })
  })
}

function checkGuest(g: GuestInput, field: string, collect: boolean): GuestInput {
  const name = (g?.name ?? '').replace(/\s+/g, ' ').trim()
  if (!name || name.length > 120) throw invalidField(`${field}name`, 'required, at most 120 characters')
  if (!(g.plus_n >= 0 && g.plus_n <= 10)) throw invalidField(`${field}plus_n`, '0 to 10')
  if (g.status && !STATUSES.includes(g.status)) throw invalidField(`${field}status`, STATUSES.join(', '))
  const email = g.email?.trim() ?? ''
  const phone = g.phone?.trim() ?? ''
  if (!collect && email) throw invalidField(`${field}email`, 'this list is name-only; turn on contact details for the list first')
  if (!collect && phone) throw invalidField(`${field}phone`, 'this list is name-only; turn on contact details for the list first')
  if (email && !/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(email)) throw invalidField(`${field}email`, 'not an email address')
  return { ...g, name, email, phone, note: g.note?.trim() ?? '' }
}

function quotaCheck(a: AllocRow, requested: number, exclude?: string) {
  const used = headsHeld(guests.filter(g => g.allocation_id === a.id && g.id !== exclude))
  if (used + requested > a.quota) {
    throw guestErr(409, { error: 'quota_exceeded', allocation_id: a.id, label: a.label, quota: a.quota, used, requested })
  }
}

export function addGuests(eventId: string, b: AddGuestsInput): AddResult {
  findEvent(eventId)
  assertNotPurged(eventId)
  const list = findList(eventId, b.list_id)
  if (!b.guests?.length || b.guests.length > 500) throw invalidField('guests', '1 to 500 per request')
  const input = b.guests.map((g, i) => checkGuest(g, `guests[${i}].`, list.collect_contact))
  const alloc = b.allocation_id ? allocations.find(a => a.id === b.allocation_id && a.list_id === list.id) : null
  if (b.allocation_id && !alloc) throw invalidField('allocation_id', 'not an allocation of this list')
  if (alloc) {
    const state = allocationState(alloc)
    if (state === 'revoked') throw guestErr(409, { error: 'allocation_revoked' })
    if (state === 'closed') throw guestErr(409, { error: 'allocation_closed' })
    input.forEach((g, i) => {
      if (g.plus_n > alloc.plus_n_max) throw invalidField(`guests[${i}].plus_n`, 'more than this allocation allows')
    })
  }
  const seen = new Set<string>()
  for (const g of guests.filter(x => x.event_id === eventId)) {
    if (g.email) seen.add(`e:${g.email.toLowerCase()}`)
    else if (g.list_id === list.id) seen.add(`n:${fold(g.name)}`)
  }
  const duplicates: number[] = []
  const fresh: (GuestInput & { status: GuestStatus })[] = []
  input.forEach((g, i) => {
    const key = g.email ? `e:${g.email.toLowerCase()}` : `n:${fold(g.name)}`
    if (seen.has(key)) {
      duplicates.push(i)
      return
    }
    seen.add(key)
    fresh.push({ ...g, status: g.status || (alloc?.requires_approval ? 'pending' : 'going') })
  })
  if (alloc) quotaCheck(alloc, headsHeld(fresh))
  const now = new Date().toISOString()
  const added = fresh.map((g): GuestRow => ({
    id: newId('g'), event_id: eventId, list_id: list.id, allocation_id: alloc?.id ?? null, name: g.name, email: g.email ?? '',
    phone: g.phone ?? '', note: g.note ?? '', plus_n: g.plus_n, status: g.status, source: b.source ?? 'manual', created_at: now, updated_at: now,
  }))
  guests.push(...added)
  return { added: added.map(g => guestView(g)), duplicates }
}

export function updateGuest(eventId: string, id: string, b: GuestInput): Guest {
  const g = guests.find(x => x.id === id && x.event_id === eventId)
  if (!g) throw guestErr(404, { error: 'not_found' })
  if (g.purged) throw eventPurged()
  const list = findList(eventId, g.list_id)
  const next = checkGuest({ ...b, status: b.status || g.status }, '', list.collect_contact)
  const status = next.status ?? g.status
  const alloc = allocations.find(a => a.id === g.allocation_id)
  if (alloc) {
    if (next.plus_n > alloc.plus_n_max && next.plus_n > g.plus_n) throw invalidField('plus_n', 'more than this allocation allows')
    const before = holdsQuota(g.status) ? 1 + g.plus_n : 0
    const after = holdsQuota(status) ? 1 + next.plus_n : 0
    if (after > before) quotaCheck(alloc, after, g.id)
  }
  Object.assign(g, { name: next.name, email: next.email, phone: next.phone, note: next.note, plus_n: next.plus_n, status, updated_at: new Date().toISOString() })
  return guestView(g)
}

export function deleteGuest(eventId: string, id: string) {
  const i = guests.findIndex(x => x.id === id && x.event_id === eventId)
  if (i < 0) throw guestErr(404, { error: 'not_found' })
  guests.splice(i, 1)
}

export function bulkStatus(eventId: string, b: BulkStatusInput): BulkResult {
  findEvent(eventId)
  if (!STATUSES.includes(b.status)) throw invalidField('status', STATUSES.join(', '))
  const emails = [...new Map((b.emails ?? []).map(e => [e.trim().toLowerCase(), e.trim()])).entries()].filter(([k]) => k)
  if (!emails.length && !b.guest_ids?.length) throw invalidField('emails', 'paste at least one email or pick guests')
  const mine = guests.filter(g => g.event_id === eventId)
  const targets = mine.filter(g => b.guest_ids?.includes(g.id) || (g.email && emails.some(([k]) => k === g.email.toLowerCase())))
  const unmatched = emails.filter(([k]) => !mine.some(g => g.email.toLowerCase() === k)).map(([, e]) => e)
  const changed = targets.filter(g => g.status !== b.status)
  const delta = new Map<string, number>()
  for (const g of changed) {
    if (g.allocation_id && holdsQuota(b.status) && !holdsQuota(g.status)) delta.set(g.allocation_id, (delta.get(g.allocation_id) ?? 0) + 1 + g.plus_n)
  }
  for (const [id, heads] of delta) quotaCheck(allocations.find(a => a.id === id)!, heads)
  for (const g of changed) Object.assign(g, { status: b.status, updated_at: new Date().toISOString() })
  return { matched: targets.length, updated: changed.length, unmatched }
}

/** Check-in state of an event's guests and tickets, derived from the door rows (undone rows never count). */
const eventArrivals = (eventId: string) => arrivalsFrom(checkinRows.filter(c => c.event_id === eventId))

/** A guest as the API shows it: with its live check-in state. */
function guestView(g: GuestRow, arrivals = eventArrivals(g.event_id)): Guest {
  const a = arrivals.get(`guest:${g.id}`)
  return { ...strip(g), heads_in: a?.heads_in ?? 0, first_in_at: a?.first_in_at ?? null }
}

/**
 * The guest table. status is a guest status or 'checked_in' (heads in ≥ 1,
 * any status); the CSV export refuses 'checked_in' (checkedIn = false).
 */
export function guestPage(eventId: string, filter: { status?: string, list_id?: string }, checkedIn = true): GuestPage {
  findEvent(eventId)
  const allowed: string[] = checkedIn ? [...STATUSES, 'checked_in'] : STATUSES
  if (filter.status && !allowed.includes(filter.status)) throw invalidField('status', allowed.join(', '))
  const arrivals = eventArrivals(eventId)
  const inList = guests.filter(g => g.event_id === eventId && (!filter.list_id || g.list_id === filter.list_id)).map(g => guestView(g, arrivals))
  const counts = { ...countByStatus(inList), tickets: filter.list_id ? 0 : validTickets(eventId) }
  const match = (g: Guest) => !filter.status || (filter.status === 'checked_in' ? g.heads_in >= 1 : g.status === filter.status)
  return {
    guests: inList.filter(match),
    tickets: filter.status || filter.list_id ? [] : ticketViews(eventId),
    counts,
  }
}

export function overviewRows(): OverviewRow[] {
  return events.filter(e => Date.parse(e.ends_at) > Date.now() && e.status !== 'cancelled')
    .sort((a, b) => a.starts_at.localeCompare(b.starts_at)).map((e) => {
      const mine = guests.filter(g => g.event_id === e.id)
      const allocs = allocations.filter(a => a.event_id === e.id && !a.revoked_at)
      return {
        event_id: e.id, title: e.title, status: e.status, starts_at: e.starts_at, timezone: e.timezone, capacity: e.capacity,
        lists: guestLists.filter(l => l.event_id === e.id).length, guests: mine.filter(g => g.status !== 'declined').length,
        going_heads: mine.filter(g => g.status === 'going').reduce((n, g) => n + 1 + g.plus_n, 0),
        pending: mine.filter(g => g.status === 'pending').length,
        used: headsHeld(mine.filter(g => allocs.some(a => a.id === g.allocation_id))), quota: allocs.reduce((n, a) => n + a.quota, 0),
        tickets: validTickets(e.id),
      }
    })
}

// ---------------------------------------------------------------- attendees (P2.2)
// Mirrors api/internal/promoter/guest/import_service.go: orders keyed by
// (event, source, order ref), tickets by ticket id, then barcode, then the
// n-th ticket of the order. The mock keeps barcodes in the clear; the
// server stores only ciphertext and a blind index.

interface TicketTypeRow { id: string, event_id: string, name: string, key: string, ref: string | null }
interface OrderRow { id: string, event_id: string, source: ImportPreset, ref: string, buyer_name: string, buyer_email: string }
interface PositionRow {
  id: string, event_id: string, order_id: string, ticket_type_id: string, ref: string, name: string, email: string, secret: string,
  status: TicketStatus, imported_at: string, purged?: boolean
}

export const ticketTypes: TicketTypeRow[] = [
  { id: 'tt-early', event_id: 'e-klubnacht', name: 'Early bird', key: 'early bird', ref: null },
  { id: 'tt-regular', event_id: 'e-klubnacht', name: 'Regular', key: 'regular', ref: null },
  { id: 'tt-02-early', event_id: 'e-klubnacht-02', name: 'Early bird', key: 'early bird', ref: null },
  { id: 'tt-02-regular', event_id: 'e-klubnacht-02', name: 'Regular', key: 'regular', ref: null },
  { id: 'tt-01-regular', event_id: 'e-klubnacht-01', name: 'Regular', key: 'regular', ref: null },
]
export const orders: OrderRow[] = [
  { id: 'o-1', event_id: 'e-klubnacht', source: 'dice', ref: 'D-7001', buyer_name: 'Hana Kim', buyer_email: 'hana@example.org' },
  { id: 'o-2', event_id: 'e-klubnacht', source: 'dice', ref: 'D-7002', buyer_name: 'Theo Brandt', buyer_email: 'theo@example.org' },
  { id: 'o-02-1', event_id: 'e-klubnacht-02', source: 'dice', ref: 'D-6001', buyer_name: 'Nils Berg', buyer_email: 'nils@example.org' },
  { id: 'o-01-1', event_id: 'e-klubnacht-01', source: 'ra', ref: 'RA-5001', buyer_name: 'Eva Lind', buyer_email: 'eva@example.org' },
]
export const positions: PositionRow[] = [
  { id: 'p-1', event_id: 'e-klubnacht', order_id: 'o-1', ticket_type_id: 'tt-early', ref: 'TK-1', name: 'Hana Kim', email: 'hana@example.org', secret: 'DICE-0001', status: 'valid', imported_at: iso(-2) },
  { id: 'p-2', event_id: 'e-klubnacht', order_id: 'o-1', ticket_type_id: 'tt-early', ref: 'TK-2', name: 'Mika Kim', email: 'hana@example.org', secret: 'DICE-0002', status: 'valid', imported_at: iso(-2) },
  { id: 'p-3', event_id: 'e-klubnacht', order_id: 'o-2', ticket_type_id: 'tt-regular', ref: 'TK-3', name: 'Theo Brandt', email: 'theo@example.org', secret: 'DICE-0003', status: 'refunded', imported_at: iso(-2) },
  { id: 'p-02-1', event_id: 'e-klubnacht-02', order_id: 'o-02-1', ticket_type_id: 'tt-02-early', ref: 'TK-1', name: 'Nils Berg', email: 'nils@example.org', secret: 'DICE-6001', status: 'valid', imported_at: iso(-9) },
  { id: 'p-02-2', event_id: 'e-klubnacht-02', order_id: 'o-02-1', ticket_type_id: 'tt-02-early', ref: 'TK-2', name: 'Ada Berg', email: 'nils@example.org', secret: 'DICE-6002', status: 'valid', imported_at: iso(-9) },
  { id: 'p-02-3', event_id: 'e-klubnacht-02', order_id: 'o-02-1', ticket_type_id: 'tt-02-regular', ref: 'TK-3', name: 'Jonas Wolf', email: 'nils@example.org', secret: 'DICE-6003', status: 'valid', imported_at: iso(-9) },
  { id: 'p-02-4', event_id: 'e-klubnacht-02', order_id: 'o-02-1', ticket_type_id: 'tt-02-regular', ref: 'TK-4', name: 'Lea Wolf', email: 'nils@example.org', secret: 'DICE-6004', status: 'refunded', imported_at: iso(-9) },
  { id: 'p-01-1', event_id: 'e-klubnacht-01', order_id: 'o-01-1', ticket_type_id: 'tt-01-regular', ref: 'TK-1', name: 'Eva Lind', email: 'eva@example.org', secret: 'RA-9001', status: 'valid', imported_at: iso(-37) },
  { id: 'p-01-2', event_id: 'e-klubnacht-01', order_id: 'o-01-1', ticket_type_id: 'tt-01-regular', ref: 'TK-2', name: 'Jan Lind', email: 'eva@example.org', secret: 'RA-9002', status: 'valid', imported_at: iso(-37) },
]

const validTickets = (eventId: string) => positions.filter(p => p.event_id === eventId && p.status === 'valid').length

function ticketViews(eventId: string): Ticket[] {
  const arrivals = eventArrivals(eventId)
  return positions.filter(p => p.event_id === eventId).map((p) => {
    const o = orders.find(x => x.id === p.order_id)!
    const firstIn = arrivals.get(`ticket:${p.id}`)?.first_in_at ?? null
    return {
      id: p.id, order_id: o.id, source: o.source, order_ref: o.ref, ticket_type_id: p.ticket_type_id,
      ticket_type: ticketTypes.find(t => t.id === p.ticket_type_id)?.name ?? '', name: p.name, email: p.email, status: p.status,
      imported_at: p.imported_at, checked_in: firstIn !== null, first_in_at: firstIn, ...(p.purged ? { purged: true } : {}),
    }
  }).sort((a, b) => a.order_ref.localeCompare(b.order_ref))
}

interface PlannedPos { id: string, ref: string, row: ImportRow, existing: PositionRow | null, type?: TicketTypeRow & { isNew?: boolean }, changed: boolean }

export function importAttendees(eventId: string, preset: ImportPreset, explicit: Partial<Record<ImportField, string>>, table: CsvTable, dryRun: boolean): ImportResult {
  findEvent(eventId)
  assertNotPurged(eventId)
  let m: ReturnType<typeof resolveMapping>
  try {
    m = resolveMapping(preset, table.headers, explicit)
  } catch (p) {
    throw guestErr(422, p as ImportProblem as unknown as Record<string, unknown>)
  }
  const res: ImportResult = {
    dry_run: dryRun, import_id: null, preset, encoding: table.encoding, mapping: m.header,
    counts: { rows: table.rows.length, orders_new: 0, orders_updated: 0, positions_new: 0, positions_updated: 0, positions_unchanged: 0, rejected: 0, not_in_file: 0, on_guest_list: 0 },
    ticket_types: [], rejected: [], preview: [],
  }
  const reject = (line: number, reason: string) => {
    res.counts.rejected++
    if (res.rejected.length < 200) res.rejected.push({ line, reason })
  }
  const rows: ImportRow[] = []
  for (const tr of table.rows) {
    const r = mapRow(m, tr)
    if (r.reason !== undefined) reject(tr.line, r.reason)
    else rows.push(r.row)
  }

  const existingOrders = new Map(orders.filter(o => o.event_id === eventId && o.source === preset).map(o => [o.ref, o]))
  const secretOwner = new Map(positions.filter(p => p.event_id === eventId && p.secret).map(p => [p.secret, p.id]))
  const groups = new Map<string, number[]>()
  rows.forEach((r, i) => {
    const ref = r.order_ref || r.ticket_ref || `~${r.secret}`
    groups.set(ref, [...(groups.get(ref) ?? []), i])
  })
  const accepted: (PlannedPos | null)[] = rows.map(() => null)
  const why: string[] = []
  const matched = new Set<string>()
  const seenSecret = new Map<string, number>()
  for (const [ref, idx] of groups) {
    const ex = existingOrders.get(ref)
    const mine = ex ? positions.filter(p => p.order_id === ex.id) : []
    const seenRef = new Map<string, number>()
    for (const i of idx) {
      const r = rows[i]!
      if (r.secret) {
        if (seenSecret.has(r.secret)) { why[i] = `same barcode as line ${seenSecret.get(r.secret)}`; continue }
        seenSecret.set(r.secret, r.line)
      }
      const p: PlannedPos = { id: '', ref: '', row: r, existing: null, changed: false }
      if (r.ticket_ref) {
        if (seenRef.has(r.ticket_ref)) { why[i] = `same ticket as line ${seenRef.get(r.ticket_ref)}`; continue }
        seenRef.set(r.ticket_ref, r.line)
        p.ref = r.ticket_ref
        p.existing = mine.find(x => x.ref === r.ticket_ref) ?? null
      } else if (r.secret) {
        const x = mine.find(y => y.secret === r.secret && !matched.has(y.id))
        if (x) { p.existing = x; p.ref = x.ref }
      }
      if (p.existing) matched.add(p.existing.id)
      accepted[i] = p
    }
    let n = 0
    for (const i of idx) {
      const p = accepted[i]
      if (!p || p.row.ticket_ref) continue
      n++
      if (p.existing) continue
      let key = `#${n}`
      const x = mine.find(y => y.ref === key && !matched.has(y.id) && (!y.secret || !p.row.secret))
      if (x) { p.existing = x; p.ref = key; matched.add(x.id); continue }
      for (let k = n; ; k++) {
        key = `#${k}`
        if (!mine.some(y => y.ref === key) && !seenRef.has(key)) break
      }
      seenRef.set(key, p.row.line)
      p.ref = key
    }
    for (const i of idx) {
      const p = accepted[i]
      if (!p?.row.secret) continue
      const owner = secretOwner.get(p.row.secret)
      if (owner && owner !== p.existing?.id) {
        why[i] = 'barcode already belongs to another ticket of this event'
        if (p.existing) matched.delete(p.existing.id)
        accepted[i] = null
      }
    }
  }

  const typesUsed: (TicketTypeRow & { isNew?: boolean, tickets: number, valid: number })[] = []
  const pending: (TicketTypeRow & { isNew?: boolean })[] = []
  const eventTypes = () => [...ticketTypes.filter(t => t.event_id === eventId), ...pending]
  accepted.forEach((p, i) => {
    const r = rows[i]!
    if (!p) { reject(r.line, why[i]!); return }
    const key = fold(r.ticket_type)
    type Planned = TicketTypeRow & { isNew?: boolean }
    let t: Planned | undefined = r.ticket_type_ref ? eventTypes().find(x => x.ref === r.ticket_type_ref) : undefined
    t ??= eventTypes().find(x => x.key === key)
    if (!t) {
      t = { id: newId('tt'), event_id: eventId, name: r.ticket_type, key, ref: r.ticket_type_ref || null, isNew: true }
      pending.push(t)
    }
    const typeId = t.id
    let used = typesUsed.find(x => x.id === typeId)
    if (!used) {
      used = { ...t, tickets: 0, valid: 0 }
      typesUsed.push(used)
    }
    used.tickets++
    if (r.status === 'valid') used.valid++
    p.type = t
    p.id = p.existing?.id ?? newId('p')
    const e = p.existing
    p.changed = !!e && (e.ticket_type_id !== t.id || e.status !== r.status || e.name !== r.name || e.email.toLowerCase() !== r.email.toLowerCase()
      || (!!r.secret && e.secret !== r.secret))
  })
  const guestEmails = new Set(guests.filter(g => g.event_id === eventId && g.email).map(g => g.email.toLowerCase()))
  res.counts.on_guest_list = accepted.filter((p, i) => p && rows[i]!.email && guestEmails.has(rows[i]!.email.toLowerCase())).length

  const plannedOrders: { ref: string, existing: OrderRow | null, changed: boolean, positions: PlannedPos[] }[] = []
  for (const [ref, idx] of groups) {
    const ps = idx.map(i => accepted[i]).filter((p): p is PlannedPos => !!p)
    if (!ps.length) continue
    const ex = existingOrders.get(ref) ?? null
    const first = ps[0]!.row
    const changed = !!ex && (ex.buyer_name !== first.buyer_name || ex.buyer_email.toLowerCase() !== first.buyer_email.toLowerCase())
    if (!ex) res.counts.orders_new++
    else if (changed) res.counts.orders_updated++
    for (const p of ps) {
      if (!p.existing) res.counts.positions_new++
      else if (p.changed) res.counts.positions_updated++
      else res.counts.positions_unchanged++
    }
    plannedOrders.push({ ref, existing: ex, changed, positions: ps })
  }
  const existingCount = positions.filter(p => [...existingOrders.values()].some(o => o.id === p.order_id)).length
  res.counts.not_in_file = existingCount - [...matched].length
  res.rejected.sort((a, b) => a.line - b.line)
  res.ticket_types = typesUsed.map(t => ({ name: t.name, new: !!t.isNew, tickets: t.tickets, valid: t.valid }))
  res.preview = accepted.flatMap((p, i) => (p ? [{
    line: rows[i]!.line, order_ref: rows[i]!.order_ref || rows[i]!.ticket_ref, name: maskName(p.row.name), email: maskEmail(p.row.email),
    ticket_type: p.type!.name, status: p.row.status, action: (p.existing ? (p.changed ? 'update' : 'unchanged') : 'new') as 'new' | 'update' | 'unchanged',
  }] : [])).slice(0, 10)
  if (dryRun) return res

  const now = new Date().toISOString()
  ticketTypes.push(...pending.map(({ isNew: _n, ...t }) => t))
  for (const o of plannedOrders) {
    const first = o.positions[0]!.row
    let order = o.existing
    if (!order) {
      order = { id: newId('o'), event_id: eventId, source: preset, ref: o.ref, buyer_name: first.buyer_name, buyer_email: first.buyer_email }
      orders.push(order)
    } else if (o.changed) {
      Object.assign(order, { buyer_name: first.buyer_name, buyer_email: first.buyer_email })
    }
    for (const p of o.positions) {
      const r = p.row
      if (!p.existing) {
        positions.push({ id: p.id, event_id: eventId, order_id: order.id, ticket_type_id: p.type!.id, ref: p.ref, name: r.name, email: r.email, secret: r.secret, status: r.status, imported_at: now })
      } else if (p.changed) {
        Object.assign(p.existing, { ticket_type_id: p.type!.id, name: r.name, email: r.email, secret: r.secret || p.existing.secret, status: r.status, imported_at: now })
      }
    }
  }
  res.import_id = newId('imp')
  return res
}

// ---------------------------------------------------------------- door (P2.3)
// Mirrors the P2.3 contract (api/internal/promoter/door + identity). Mock rules:
// - Door login accepts ANY 6-digit PIN except '000000' (which fails like a
//   wrong PIN), for any registered, unrevoked device token. The session is a
//   cookie `klubhub_door_mock` naming the device and event. Two more magic
//   PINs mirror the Go API's lockout and expiry answers: '111111' → 429
//   {error:'pin_locked', retry_after: now + 15 min} (the event's staff PIN
//   status then shows locked_until until a new staff PIN is generated; other
//   PINs still log in, since parallel e2e runs share this mock);
//   '999999' → 401 {error:'pin_expired'}.
// - Generating a staff PIN returns a random 6-digit PIN; the manager PIN is
//   always MOCK_MANAGER_PIN ('246810'), and the bundle carries a real
//   PBKDF2-SHA256 verifier for it (210 000 iterations) so the offline check
//   runs end to end. e-klubnacht starts with that manager PIN set.
// - The sync cursor is opaque (base64url of a sequence number); inserts and
//   undos both bump a row's sequence, so undos from other devices reach
//   every door. Rejected ops are not stored: a re-send is evaluated again.
// - Rejection codes match the Go API: unknown_subject, unknown_target,
//   unknown_list, id_conflict, manager_pin_invalid, "invalid_op: …",
//   "invalid_add: …".

export const MOCK_MANAGER_PIN = '246810'
export const MOCK_BAD_PIN = '000000'
export const MOCK_LOCKED_PIN = '111111'
export const MOCK_EXPIRED_PIN = '999999'
export const DOOR_COOKIE = 'klubhub_door_mock'
const PIN_WINDOW_MS = 36 * 3_600_000

/** public_key (P2.6): base64url X25519 key sent at registration; the device list does not return it (as the API). */
interface DeviceRow extends DoorDevice { token: string, public_key: string | null }
export const doorDevices: DeviceRow[] = [
  { id: 'dd-front', label: 'Front door phone', token: 'mock-door-token-front', created_at: iso(-5), last_seen_at: null, revoked_at: null, public_key: null },
]

interface PinRow { pin: string, valid_until: string, verifier?: ManagerPinVerifier, locked_until?: string }
const doorPins = new Map<string, { staff?: PinRow, manager?: PinRow }>([
  ['e-klubnacht', { manager: { pin: MOCK_MANAGER_PIN, valid_until: iso(6, 14) } }],
])

interface DoorSessionRow { device_id: string, event_id: string, expires_at: string }
const doorSessions = new Map<string, DoorSessionRow>()

interface CheckinRow extends DoorCheckin { event_id: string, seq: number }
interface CounterRow { event_id: string, nonce: string, kind: 'walkup' | 'in' | 'out', delta: number, at: string, device_id: string, undone: boolean, seq: number }
const checkinRows: CheckinRow[] = []
const counterRows: CounterRow[] = []
const opResults = new Map<string, OpResult>()
const addNonces = new Map<string, string>()
let doorSeq = 0

const unauthorized = () => createError({ statusCode: 401, data: { error: 'invalid_credentials' } })

function verifierFor(pin: string): ManagerPinVerifier {
  const salt = randomBytes(16)
  const iterations = 210_000
  return { salt: salt.toString('base64'), iterations, hash: pbkdf2Sync(pin, salt, iterations, 32, 'sha256').toString('base64') }
}

export const deviceList = (): DoorDevice[] => doorDevices.map(({ token: _t, public_key: _k, ...d }) => d)

export function registerDoorDevice(label: string, publicKey?: unknown): RegisteredDevice {
  const l = (label ?? '').trim()
  if (!l || l.length > 80) throw guestErr(400, { error: 'invalid_input' })
  let pub: string | null = null
  if (publicKey !== undefined && publicKey !== null) {
    if (typeof publicKey !== 'string' || !/^[\w-]+$/.test(publicKey) || Buffer.from(publicKey, 'base64url').length !== 32) throw invalidField('public_key', '32-byte X25519 key, base64url')
    pub = publicKey
  }
  const d: DeviceRow = { id: newId('dd'), label: l, token: `mock-door-token-${randomBytes(12).toString('hex')}`, created_at: new Date().toISOString(), last_seen_at: null, revoked_at: null, public_key: pub }
  doorDevices.push(d)
  return { id: d.id, label: d.label, token: d.token }
}

export function revokeDoorDevice(id: string) {
  const d = doorDevices.find(x => x.id === id)
  if (d && !d.revoked_at) d.revoked_at = new Date().toISOString()
}

export function setDoorPin(eventId: string, b: { manager?: boolean, valid_until?: string }): { pin: string, valid_until: string, manager: boolean } {
  findEvent(eventId)
  assertNotPurged(eventId)
  const until = Date.parse(b?.valid_until ?? '')
  const now = Date.now()
  if (!(until > now) || until - now > PIN_WINDOW_MS) throw guestErr(400, { error: 'invalid_input' })
  const manager = !!b.manager
  let pin = MOCK_MANAGER_PIN
  if (!manager) {
    do pin = String(randomInt(0, 1_000_000)).padStart(6, '0')
    while ([MOCK_BAD_PIN, MOCK_MANAGER_PIN, MOCK_LOCKED_PIN, MOCK_EXPIRED_PIN].includes(pin))
  }
  const row: PinRow = { pin, valid_until: new Date(until).toISOString() }
  doorPins.set(eventId, { ...doorPins.get(eventId), [manager ? 'manager' : 'staff']: row })
  return { pin, valid_until: row.valid_until, manager }
}

export function doorPinStatus(eventId: string): PinStatus {
  findEvent(eventId)
  const p = doorPins.get(eventId) ?? {}
  const live = (r?: PinRow) => (r && Date.parse(r.valid_until) > Date.now()
    ? { valid_until: r.valid_until, locked_until: r.locked_until && Date.parse(r.locked_until) > Date.now() ? r.locked_until : null }
    : null)
  return { staff: live(p.staff), manager: live(p.manager) }
}

/** Door login: returns the session id for the mock cookie. */
export function doorLogin(b: { device_token?: string, event_id?: string, pin?: string }): { session: string, expires_at: string } {
  const d = doorDevices.find(x => x.token === b?.device_token && !x.revoked_at)
  const pin = String(b?.pin ?? '').trim()
  if (!d || !/^\d{6}$/.test(pin) || pin === MOCK_BAD_PIN || !events.some(e => e.id === b.event_id)) throw unauthorized()
  const staff = doorPins.get(b.event_id!)?.staff
  if (pin === MOCK_EXPIRED_PIN) throw createError({ statusCode: 401, data: { error: 'pin_expired' } })
  if (pin === MOCK_LOCKED_PIN) {
    // Only this PIN answers "locked" (parallel e2e runs share the mock); the status still shows the lock.
    const until = new Date(Date.now() + 15 * 60_000).toISOString()
    if (staff) staff.locked_until = until
    throw createError({ statusCode: 429, data: { error: 'pin_locked', retry_after: until } })
  }
  const expires = staff && Date.parse(staff.valid_until) > Date.now() ? staff.valid_until : new Date(Date.now() + 12 * 3_600_000).toISOString()
  d.last_seen_at = new Date().toISOString()
  const session = randomBytes(16).toString('hex')
  doorSessions.set(session, { device_id: d.id, event_id: b.event_id!, expires_at: expires })
  return { session, expires_at: expires }
}

export function doorSession(cookie: string | undefined): DoorSessionRow {
  const s = cookie ? doorSessions.get(cookie) : undefined
  const d = s && doorDevices.find(x => x.id === s.device_id)
  if (!s || !d || d.revoked_at || Date.parse(s.expires_at) <= Date.now()) {
    throw createError({ statusCode: 403, data: { error: 'door_session_required' } })
  }
  return s
}

const encodeCursor = (seq: number) => Buffer.from(`c${seq}`).toString('base64url')
function decodeCursor(c: string | null | undefined): number {
  if (!c) return 0
  const m = /^c(\d+)$/.exec(Buffer.from(c, 'base64url').toString())
  if (!m) throw guestErr(422, { error: 'invalid', field: 'since', problem: 'unknown cursor' })
  return Number(m[1])
}

function eventCheckins(eventId: string, since = 0): DoorCheckin[] {
  return checkinRows.filter(c => c.event_id === eventId && c.seq > since)
    .map(({ event_id: _e, seq: _s, ...c }) => c)
}

function eventCounters(eventId: string): DoorCounters {
  const c: DoorCounters = { walkups: 0, manual_in: 0, manual_out: 0 }
  for (const r of counterRows) {
    if (r.event_id !== eventId || r.undone) continue
    if (r.kind === 'walkup') c.walkups += r.delta
    else if (r.kind === 'in') c.manual_in += r.delta
    else c.manual_out += r.delta
  }
  return c
}

export function doorBundle(s: DoorSessionRow): DoorBundle {
  const e = findEvent(s.event_id)
  assertNotPurged(e.id)
  const mgr = doorPins.get(e.id)?.manager
  const liveMgr = mgr && Date.parse(mgr.valid_until) > Date.now() ? mgr : null
  if (liveMgr && !liveMgr.verifier) liveMgr.verifier = verifierFor(liveMgr.pin)
  return {
    generated_at: new Date().toISOString(), device_id: s.device_id, session_expires_at: s.expires_at,
    event: { id: e.id, title: e.title, starts_at: e.starts_at, ends_at: e.ends_at, doors_at: e.doors_at, timezone: e.timezone, capacity: e.capacity },
    lists: guestLists.filter(l => l.event_id === e.id).sort((a, b) => a.position - b.position).map(l => ({
      id: l.id, name: l.name, type: l.type,
      entry_terms: { price_mode: l.entry_terms.price_mode, reduced_price_text: l.entry_terms.reduced_price_text, cutoff_at: l.entry_terms.cutoff_at ?? null, perks: l.entry_terms.perks },
    })),
    // Name-only: no email or phone ever reaches the door.
    guests: guests.filter(g => g.event_id === e.id && g.status !== 'declined')
      .map(g => ({ id: g.id, list_id: g.list_id, name: g.name, plus_n: g.plus_n, status: g.status, note: g.note })),
    tickets: positions.filter(p => p.event_id === e.id).map((p) => {
      const o = orders.find(x => x.id === p.order_id)!
      return { id: p.id, name: p.name, ticket_type: ticketTypes.find(t => t.id === p.ticket_type_id)?.name ?? '', order_ref: o.ref, source: o.source, status: p.status, secret: p.secret }
    }),
    checkins: eventCheckins(e.id),
    counters: eventCounters(e.id),
    cursor: encodeCursor(doorSeq),
    manager_pin: liveMgr?.verifier ?? null,
  }
}

function subjectOf(eventId: string, s: DoorSubject | undefined): number | null {
  if (s?.kind === 'guest') {
    const g = guests.find(x => x.id === s.id && x.event_id === eventId && x.status !== 'declined')
    return g ? 1 + g.plus_n : null
  }
  if (s?.kind === 'ticket') return positions.some(p => p.id === s.id && p.event_id === eventId) ? 1 : null
  return null
}

function applyOp(sess: DoorSessionRow, op: DoorOp): OpResult {
  const nonce = String(op?.nonce ?? '')
  if (nonce.length < 8 || nonce.length > 64) return { nonce, status: 'rejected', conflict: false, error: 'invalid_op: nonce must be 8 to 64 characters' }
  const prev = opResults.get(nonce)
  if (prev) return { ...prev, status: 'duplicate' }
  const at = op.at && !Number.isNaN(Date.parse(op.at)) ? op.at : new Date().toISOString()
  let res: OpResult
  if (op.type === 'checkin') {
    const allow = subjectOf(sess.event_id, op.subject)
    if (allow === null) res = { nonce, status: 'rejected', conflict: false, error: 'unknown_subject' }
    else if (!(op.count >= 1 && op.count <= 50) || (op.direction !== 'in' && op.direction !== 'out')) res = { nonce, status: 'rejected', conflict: false, error: 'invalid_op: count 1 to 50, direction in or out' }
    else {
      const key = `${op.subject.kind}:${op.subject.id}`
      const live = checkinRows.filter(c => c.event_id === sess.event_id && !c.undone && `${c.subject.kind}:${c.subject.id}` === key)
      const heads = live.reduce((n, c) => n + (c.direction === 'in' ? c.count : -c.count), 0)
      const conflict = op.direction === 'in' && heads + op.count > allow && live.some(c => c.direction === 'in' && c.device_id !== sess.device_id)
      checkinRows.push({
        event_id: sess.event_id, seq: ++doorSeq, nonce, subject: { kind: op.subject.kind, id: op.subject.id }, count: op.count,
        direction: op.direction, at, device_id: sess.device_id, undone: false, conflict,
      })
      res = { nonce, status: 'applied', conflict }
    }
  } else if (op.type === 'undo') {
    const c = checkinRows.find(x => x.nonce === op.target && x.event_id === sess.event_id)
    const k = counterRows.find(x => x.nonce === op.target && x.event_id === sess.event_id)
    // Undoing an already-undone row is a no-op that still applies.
    if (c && !c.undone) Object.assign(c, { undone: true, seq: ++doorSeq })
    if (k && !k.undone) Object.assign(k, { undone: true, seq: ++doorSeq })
    res = c || k ? { nonce, status: 'applied', conflict: false } : { nonce, status: 'rejected', conflict: false, error: 'unknown_target' }
  } else if (op.type === 'counter') {
    if (!['walkup', 'in', 'out'].includes(op.kind) || !(op.delta >= 1 && op.delta <= 50)) res = { nonce, status: 'rejected', conflict: false, error: 'invalid_op: kind walkup, in or out; delta 1 to 50' }
    else {
      counterRows.push({ event_id: sess.event_id, nonce, kind: op.kind, delta: op.delta, at, device_id: sess.device_id, undone: false, seq: ++doorSeq })
      res = { nonce, status: 'applied', conflict: false }
    }
  } else {
    res = { nonce, status: 'rejected', conflict: false, error: 'invalid_op: unknown type' }
  }
  if (res.status !== 'rejected') opResults.set(nonce, res)
  return res
}

export function doorSync(sess: DoorSessionRow, b: { since?: string | null, ops?: DoorOp[] }): SyncResponse {
  const ops = b?.ops ?? []
  if (ops.length > 500) throw guestErr(422, { error: 'invalid', field: 'ops', problem: 'at most 500 per request' })
  const since = decodeCursor(b?.since)
  const results = ops.map(op => applyOp(sess, op))
  return { results, checkins: eventCheckins(sess.event_id, since), counters: eventCounters(sess.event_id), cursor: encodeCursor(doorSeq) }
}

export function doorAdds(sess: DoorSessionRow, b: { adds?: DoorAdd[] }): { results: DoorAddResult[] } {
  const adds = b?.adds ?? []
  assertNotPurged(sess.event_id)
  if (!adds.length || adds.length > 50) throw guestErr(422, { error: 'invalid', field: 'adds', problem: '1 to 50 per request' })
  const mgr = doorPins.get(sess.event_id)?.manager
  const results = adds.map((a): DoorAddResult => {
    const base = { nonce: a.nonce, id: a.id }
    if (addNonces.get(a.nonce) === a.id) return { ...base, status: 'duplicate' }
    if (guests.some(g => g.id === a.id)) return { ...base, status: 'rejected', error: 'id_conflict' }
    if (!mgr || Date.parse(mgr.valid_until) <= Date.now() || a.manager_pin !== mgr.pin) return { ...base, status: 'rejected', error: 'manager_pin_invalid' }
    const name = (a.name ?? '').replace(/\s+/g, ' ').trim()
    if (!name || name.length > 120 || !(a.plus_n >= 0 && a.plus_n <= 10)) return { ...base, status: 'rejected', error: 'invalid_add: name 1 to 120 characters, plus_n 0 to 10' }
    if (!guestLists.some(l => l.id === a.list_id && l.event_id === sess.event_id)) return { ...base, status: 'rejected', error: 'unknown_list' }
    addNonces.set(a.nonce, a.id)
    const now = new Date().toISOString()
    guests.push({
      id: a.id, event_id: sess.event_id, list_id: a.list_id, allocation_id: null, name, email: '', phone: '', note: '', plus_n: a.plus_n,
      status: 'going', source: 'door', created_at: now, updated_at: now,
    })
    return { ...base, status: 'applied' }
  })
  return { results }
}

// ---------------------------------------------------------------- report (P2.4)
// Mirrors the P2.4 contract: read-only aggregates over guests, allocations,
// tickets, check-ins and door counters; undone rows never count. Names only
// leave through the per-allocation list-back CSV (no email or phone).

// Last week's night (e-klubnacht-02, doors 23:00 local): arrivals from
// 23:10, a peak after 01:00, a few leaving; one undone check-in and one undone walk-up.
function seedLastNight() {
  const dev = 'dd-front'
  const seed = (nonce: string, kind: 'guest' | 'ticket', id: string, count: number, direction: 'in' | 'out', h: number, m: number, undone = false) =>
    checkinRows.push({ event_id: 'e-klubnacht-02', seq: ++doorSeq, nonce, subject: { kind, id }, count, direction, at: iso(-7, h, m), device_id: dev, undone, conflict: false })
  const count = (nonce: string, kind: 'walkup' | 'in' | 'out', delta: number, h: number, m: number, undone = false) =>
    counterRows.push({ event_id: 'e-klubnacht-02', nonce, kind, delta, at: iso(-7, h, m), device_id: dev, undone, seq: ++doorSeq })
  seed('seed-02-c01', 'ticket', 'p-02-1', 1, 'in', 0, 10)
  seed('seed-02-c02', 'guest', 'g02-6', 2, 'in', 0, 20)
  seed('seed-02-c03', 'guest', 'g02-2', 1, 'in', 0, 40, true)
  seed('seed-02-c04', 'guest', 'g02-2', 1, 'in', 0, 41)
  seed('seed-02-c05', 'guest', 'g02-1', 2, 'in', 1, 5)
  seed('seed-02-c06', 'ticket', 'p-02-3', 1, 'in', 1, 12)
  seed('seed-02-c07', 'guest', 'g02-4', 2, 'in', 1, 35)
  seed('seed-02-c08', 'guest', 'g02-9', 1, 'in', 1, 50)
  seed('seed-02-c09', 'guest', 'g02-8', 1, 'in', 2, 5)
  seed('seed-02-c10', 'guest', 'g02-9', 1, 'out', 3, 20)
  count('seed-02-k01', 'walkup', 2, 0, 55)
  count('seed-02-k02', 'walkup', 1, 1, 40)
  count('seed-02-k03', 'walkup', 1, 1, 42, true)
  count('seed-02-k04', 'walkup', 3, 2, 10)
  count('seed-02-k05', 'out', 2, 3, 30)
}
seedLastNight()

interface Tally { going: number, arrived: number, no_show_rate: number | null, heads_expected: number, heads_admitted: number, plus_ones_allowed: number, plus_ones_used: number }

function reportCheckins(eventId: string) {
  const live = checkinRows.filter(c => c.event_id === eventId && !c.undone)
  const heads = new Map<string, number>()
  const firstIn = new Map<string, string>()
  for (const c of live) {
    if (c.direction !== 'in') continue
    const key = `${c.subject.kind}:${c.subject.id}`
    heads.set(key, (heads.get(key) ?? 0) + c.count)
    if (!firstIn.has(key) || c.at < firstIn.get(key)!) firstIn.set(key, c.at)
  }
  return { live, heads, firstIn }
}

function tally(rows: GuestRow[], heads: Map<string, number>): Tally {
  const going = rows.filter(g => g.status === 'going')
  const admitted = (g: GuestRow) => heads.get(`guest:${g.id}`) ?? 0
  const arrived = rows.filter(g => admitted(g) >= 1)
  const goingArrived = going.filter(g => admitted(g) >= 1).length
  return {
    going: going.length, arrived: arrived.length,
    no_show_rate: going.length ? Math.round(((going.length - goingArrived) / going.length) * 10_000) / 10_000 : null,
    heads_expected: going.reduce((n, g) => n + 1 + g.plus_n, 0), heads_admitted: rows.reduce((n, g) => n + admitted(g), 0),
    plus_ones_allowed: going.reduce((n, g) => n + g.plus_n, 0),
    plus_ones_used: arrived.reduce((n, g) => n + Math.min(g.plus_n, admitted(g) - 1), 0),
  }
}

function reportCurve(eventId: string, tz: string, live: CheckinRow[]): CurvePoint[] {
  const moves = [
    ...live.map(c => ({ at: Date.parse(c.at), in: c.direction === 'in' ? c.count : 0, out: c.direction === 'out' ? c.count : 0, walkups: 0 })),
    ...counterRows.filter(k => k.event_id === eventId && !k.undone).map(k => ({
      at: Date.parse(k.at), in: k.kind === 'in' ? k.delta : 0, out: k.kind === 'out' ? k.delta : 0, walkups: k.kind === 'walkup' ? k.delta : 0,
    })),
  ].sort((a, b) => a.at - b.at)
  if (!moves.length) return []
  const first = bucketStart(moves[0]!.at, tz)
  const last = bucketStart(moves.at(-1)!.at, tz)
  const curve: CurvePoint[] = []
  let occupancy = 0
  let k = 0
  for (let b = first; b <= last; b += BUCKET_MS) {
    const p: CurvePoint = { bucket_start: new Date(b).toISOString(), in: 0, out: 0, walkups: 0, occupancy: 0 }
    while (k < moves.length && moves[k]!.at < b + BUCKET_MS) {
      const m = moves[k++]!
      p.in += m.in
      p.out += m.out
      p.walkups += m.walkups
    }
    occupancy += p.in + p.walkups - p.out
    p.occupancy = occupancy
    curve.push(p)
  }
  return curve
}

export function eventReport(eventId: string): EventReport {
  const e = findEvent(eventId)
  const { live, heads } = reportCheckins(e.id)
  const evGuests = guests.filter(g => g.event_id === e.id)
  const lists = guestLists.filter(l => l.event_id === e.id).sort((a, b) => a.position - b.position)
  const evTickets = positions.filter(p => p.event_id === e.id)
  const scanned = (p: PositionRow) => (heads.get(`ticket:${p.id}`) ?? 0) >= 1
  const validTicketCount = evTickets.filter(p => p.status === 'valid').length
  const all = tally(evGuests, heads)
  const curve = reportCurve(e.id, e.timezone, live)
  const peak = curve.reduce<CurvePoint | null>((best, p) => (!best || p.occupancy > best.occupancy ? p : best), null)
  const walkups = counterRows.filter(k => k.event_id === e.id && !k.undone && k.kind === 'walkup').reduce((n, k) => n + k.delta, 0)
  return {
    event: { id: e.id, title: e.title, starts_at: e.starts_at, ends_at: e.ends_at, timezone: e.timezone, capacity: e.capacity },
    generated_at: new Date().toISOString(), live: Date.now() < Date.parse(e.ends_at),
    totals: {
      guests_going: all.going, guests_arrived: all.arrived, no_show_rate: all.no_show_rate,
      heads_expected: all.heads_expected + validTicketCount,
      heads_admitted: live.filter(c => c.direction === 'in').reduce((n, c) => n + c.count, 0),
      plus_ones_allowed: all.plus_ones_allowed, plus_ones_used: all.plus_ones_used,
      tickets_valid: validTicketCount, tickets_scanned: evTickets.filter(scanned).length, walkups,
      peak_occupancy: peak && peak.occupancy > 0 ? peak.occupancy : 0, peak_at: peak && peak.occupancy > 0 ? peak.bucket_start : null,
      conflicts: live.filter(c => c.conflict).length,
    },
    by_list: lists.map(l => ({ list_id: l.id, name: l.name, type: l.type, ...tally(evGuests.filter(g => g.list_id === l.id), heads) })),
    // Artist lists first, then list position, then allocation creation (array order).
    by_submitter: allocations.filter(a => a.event_id === e.id)
      .map((a, i) => ({ a, i, l: lists.find(x => x.id === a.list_id)! }))
      .sort((x, y) => Number(y.l.type === 'artist') - Number(x.l.type === 'artist') || x.l.position - y.l.position || x.i - y.i)
      .map(({ a, l }): ReportSubmitterRow => {
        const t = tally(evGuests.filter(g => g.allocation_id === a.id), heads)
        return {
          allocation_id: a.id, list_id: l.id, list_name: l.name, list_type: l.type, submitter: a.label, quota: a.quota,
          going: t.going, arrived: t.arrived, no_show_rate: t.no_show_rate, heads_admitted: t.heads_admitted, revoked: !!a.revoked_at,
        }
      }),
    tickets_by_type: ticketTypes.filter(t => t.event_id === e.id).map(t => ({
      ticket_type_id: t.id, name: t.name,
      valid: evTickets.filter(p => p.ticket_type_id === t.id && p.status === 'valid').length,
      scanned: evTickets.filter(p => p.ticket_type_id === t.id && scanned(p)).length,
    })),
    curve,
  }
}

/**
 * One allocation's guests for the list-back CSV: name-level only, no email
 * or phone. Missing or malformed id → 422; another event's allocation → 404.
 */
export function listBackAllocation(eventId: string, allocationId: string): AllocRow {
  if (!/^[\w-]{1,64}$/.test(allocationId)) throw invalidField('allocation_id', 'an allocation id')
  const a = allocations.find(x => x.id === allocationId && x.event_id === eventId)
  if (!a) throw guestErr(404, { error: 'not_found' })
  return a
}

export function listBackRows(eventId: string, allocationId: string): (string | number)[][] {
  const e = findEvent(eventId)
  const a = listBackAllocation(e.id, allocationId)
  assertNotPurged(e.id)
  const { heads, firstIn } = reportCheckins(e.id)
  const local = (at: string | undefined) => {
    if (!at) return ''
    const z = instantToZoned(at, e.timezone)
    return `${z.date} ${z.time}`
  }
  const rows = guests.filter(g => g.event_id === e.id && g.allocation_id === a.id).sort((x, y) => x.name.localeCompare(y.name)).map((g) => {
    const n = heads.get(`guest:${g.id}`) ?? 0
    return [g.name, g.plus_n, g.status, n >= 1 ? 'yes' : 'no', n, local(firstIn.get(`guest:${g.id}`))]
  })
  return [['name', 'plus_n', 'status', 'arrived', 'heads_admitted', 'first_in_local'], ...rows]
}

// ---------------------------------------------------------------- privacy and retention (P2.5)
// Mirrors the P2.5 contract: purging anonymises in place (personal columns
// emptied, rows kept with `purged`), so counts, the curve and the report
// survive. The mock never purges on a timer; e-klubnacht-01 starts purged
// and ERASE NOW purges on request. Writes that would add personal data to
// a purged event answer 409 event_purged.

const STEP_UP_MS = 15 * 60_000

/** Org retention (missing row = 30 days). */
export const orgPrivacy = { retention_days: 30 }

interface PurgeRow { purged_at: string, trigger: 'schedule' | 'manual', counts: Record<string, number> }
const eventPurges = new Map<string, PurgeRow>()

export const eventPurged = () => guestErr(409, { error: 'event_purged' })

export function assertNotPurged(eventId: string) {
  if (eventPurges.has(eventId)) throw eventPurged()
}

/** ends_at + retention, recomputed until purged (events without an end would use starts_at + 24 h). */
function purgeAfter(e: EventDetail): string {
  const end = Date.parse(e.ends_at || '') || Date.parse(e.starts_at) + DAY
  return new Date(end + orgPrivacy.retention_days * DAY).toISOString()
}

function personalRows(eventId: string): number {
  return guests.filter(g => g.event_id === eventId && !g.purged).length
    + positions.filter(p => p.event_id === eventId && !p.purged).length
}

/** Anonymise one event in place; returns rows touched per table. */
function purge(eventId: string, trigger: PurgeRow['trigger'], at = new Date().toISOString()): PurgeRow {
  const counts = { guests: 0, orders: 0, order_positions: 0, guest_allocations: 0, door_pins: 0 }
  for (const g of guests.filter(x => x.event_id === eventId && !x.purged)) {
    Object.assign(g, { name: '', email: '', phone: '', note: '', purged: true })
    counts.guests++
  }
  for (const p of positions.filter(x => x.event_id === eventId && !x.purged)) {
    Object.assign(p, { name: '', email: '', secret: '', purged: true })
    counts.order_positions++
  }
  for (const o of orders.filter(x => x.event_id === eventId && (x.buyer_name || x.buyer_email))) {
    Object.assign(o, { buyer_name: '', buyer_email: '' })
    counts.orders++
  }
  for (const a of allocations.filter(x => x.event_id === eventId && x.submitter_contact)) {
    a.submitter_contact = ''
    counts.guest_allocations++
  }
  const pins = doorPins.get(eventId)
  if (pins) counts.door_pins = Number(!!pins.staff) + Number(!!pins.manager)
  doorPins.delete(eventId)
  const row = { purged_at: at, trigger, counts }
  eventPurges.set(eventId, row)
  return row
}

export function eventPrivacy(eventId: string) {
  const e = findEvent(eventId)
  const p = eventPurges.get(e.id)
  return { purge_after: purgeAfter(e), purged_at: p?.purged_at ?? null, retention_days: orgPrivacy.retention_days, personal_rows: personalRows(e.id) }
}

export function retentionOverview() {
  const soon = Date.now() + 7 * DAY
  return {
    retention_days: orgPrivacy.retention_days,
    upcoming: events.filter(e => e.status !== 'draft' && !eventPurges.has(e.id) && Date.parse(e.ends_at) <= soon)
      .map(e => ({ event_id: e.id, title: e.title, timezone: e.timezone, ends_at: e.ends_at, purge_after: purgeAfter(e) }))
      .sort((a, b) => a.purge_after.localeCompare(b.purge_after)).slice(0, 20),
    recent: [...eventPurges.entries()].map(([id, p]) => ({ event_id: id, title: findEvent(id).title, timezone: findEvent(id).timezone, ...p }))
      .sort((a, b) => b.purged_at.localeCompare(a.purged_at)).slice(0, 20),
  }
}

const validDays = (n: unknown): n is number => typeof n === 'number' && Number.isInteger(n) && n >= 1 && n <= 365

/** Unpurged events that `days` makes due now but the current period does not (only a shorter period can). */
function wouldPurge(days: number) {
  const now = Date.now()
  const endOf = (e: EventDetail) => Date.parse(e.ends_at || '') || Date.parse(e.starts_at) + DAY
  const current = orgPrivacy.retention_days
  const list = days >= current
    ? []
    : events.filter(e => !eventPurges.has(e.id) && endOf(e) + days * DAY <= now && endOf(e) + current * DAY > now)
      .map(e => ({ event_id: e.id, title: e.title, ends_at: e.ends_at }))
  return { would_purge: list, count: list.length }
}

/** GET /org/retention/preview?days=N */
export function retentionPreview(q: { days?: unknown }) {
  const n = Number(q?.days)
  if (!validDays(n)) throw invalidField('days', 'between 1 and 365 days')
  return wouldPurge(n)
}

/**
 * PUT /org/retention: a shorter period that makes ended events due at once
 * needs confirm_purge = their count (else 409 retention_would_purge) and a
 * recent sign-in (403 reauthentication_required), like the server. The
 * mock job never runs, so nothing is actually erased.
 */
export function setRetention(b: { retention_days?: unknown, confirm_purge?: unknown }, authAt: number) {
  const n = b?.retention_days
  if (!validDays(n)) throw invalidField('retention_days', '1 to 365 days')
  const due = wouldPurge(n)
  if (due.count > 0) {
    if (b.confirm_purge !== due.count) throw guestErr(409, { error: 'retention_would_purge', ...due })
    if (Date.now() - authAt > STEP_UP_MS) throw guestErr(403, { error: 'reauthentication_required' })
  }
  orgPrivacy.retention_days = n
  return retentionOverview()
}

/** ERASE NOW: typed title (normalised like the server), ended event, recent sign-in (mock: 15 min since this browser's mock login). */
export function purgeNow(eventId: string, b: { confirm?: string }, authAt: number) {
  const e = findEvent(eventId)
  if (Date.now() - authAt > STEP_UP_MS) throw guestErr(403, { error: 'reauthentication_required' })
  if (!confirmMatches(b?.confirm ?? '', e.title)) throw invalidField('confirm', 'type the event title')
  if (Date.parse(e.ends_at) > Date.now()) throw guestErr(409, { error: 'event_not_ended' })
  assertNotPurged(e.id)
  const p = purge(e.id, 'manual')
  return { purged_at: p.purged_at, counts: p.counts }
}

// Klubnacht 01: a few door check-ins, then erased by the schedule 30 days after it ended.
function seedPurged() {
  const e = findEvent('e-klubnacht-01')
  const seed = (nonce: string, kind: 'guest' | 'ticket', id: string, count: number, h: number, m: number) =>
    checkinRows.push({ event_id: e.id, seq: ++doorSeq, nonce, subject: { kind, id }, count, direction: 'in', at: iso(-35, h, m), device_id: 'dd-front', undone: false, conflict: false })
  seed('seed-01-c01', 'guest', 'g01-1', 2, 0, 30)
  seed('seed-01-c02', 'guest', 'g01-3', 1, 1, 10)
  seed('seed-01-c03', 'ticket', 'p-01-1', 1, 1, 20)
  purge(e.id, 'schedule', new Date(Date.parse(e.ends_at) + 30 * DAY).toISOString())
}
seedPurged()

// ---------------------------------------------------------------- P3.1: audience CRM ---

interface AudienceContactRow {
  id: string
  name: string
  email: string
  phone: string
  status: AudienceContactStatus
  source: AudienceContactSource
  consent_basis: AudienceConsentBasis
  consent_recorded_at: string
  consent_form_text: string
  double_opt_in_confirmed_at: string | null
  created_at: string
  updated_at: string
}

const AUDIENCE_STATUSES: AudienceContactStatus[] = ['active', 'unsubscribed', 'bounced', 'complained']
const AUDIENCE_SOURCES: AudienceContactSource[] = ['rsvp', 'follow', 'notify_me', 'csv', 'door']

export const audienceContactRows: AudienceContactRow[] = [
  {
    id: 'ac-1', name: 'Nadia Voss', email: 'nadia@example.com', phone: '',
    status: 'active', source: 'rsvp', consent_basis: 'consent', consent_recorded_at: iso(-40),
    consent_form_text: 'Sign up to hear about future nights', double_opt_in_confirmed_at: iso(-40, 0, 5),
    created_at: iso(-40), updated_at: iso(-40),
  },
  {
    id: 'ac-2', name: 'Lars Berg', email: 'lars@example.com', phone: '',
    status: 'active', source: 'follow', consent_basis: 'consent', consent_recorded_at: iso(-20),
    consent_form_text: 'Follow the collective', double_opt_in_confirmed_at: iso(-20, 0, 3),
    created_at: iso(-20), updated_at: iso(-20),
  },
  {
    id: 'ac-3', name: 'Priya Shah', email: 'priya@example.com', phone: '',
    status: 'unsubscribed', source: 'csv', consent_basis: 'soft_opt_in', consent_recorded_at: iso(-90),
    consent_form_text: 'Imported past-customer list', double_opt_in_confirmed_at: null,
    created_at: iso(-90), updated_at: iso(-5),
  },
]

export const audienceSegmentRows: { id: string, name: string, filter: AudienceSegmentFilter }[] = [
  { id: 'as-1', name: "RSVP'd", filter: { source: 'rsvp' } },
]

function audienceContactView(r: AudienceContactRow) {
  return { ...r, consent_ip: undefined }
}

function audienceMatches(f: { status?: string, source?: string, q?: string }, r: AudienceContactRow): boolean {
  if (f.status && r.status !== f.status) return false
  if (f.source && r.source !== f.source) return false
  if (f.q) {
    const q = f.q.toLowerCase()
    if (!r.name.toLowerCase().includes(q) && !r.email.toLowerCase().includes(q)) return false
  }
  return true
}

export function audienceCounts(): AudienceCounts {
  const c: AudienceCounts = { all: 0, active: 0, unsubscribed: 0, bounced: 0, complained: 0 }
  for (const r of audienceContactRows) {
    c.all++
    c[r.status]++
  }
  return c
}

export function audiencePage(filter: { status?: string, source?: string, q?: string }): AudiencePage {
  return {
    contacts: audienceContactRows.filter(r => audienceMatches(filter, r)).map(audienceContactView),
    counts: audienceCounts(),
  }
}

function validateAudienceInput(b: AudienceContactInput): AudienceContactInput {
  b.name = (b.name ?? '').trim()
  b.email = (b.email ?? '').trim()
  if (!b.name && !b.email) throw invalidField('name', 'a contact needs a name or an email')
  if (!AUDIENCE_SOURCES.includes(b.source)) throw invalidField('source', AUDIENCE_SOURCES.join(', '))
  if (!b.consent || !['consent', 'soft_opt_in'].includes(b.consent.basis)) throw invalidField('consent.basis', 'consent, soft_opt_in')
  return b
}

export function createAudienceContact(b: AudienceContactInput): AudienceContact {
  validateAudienceInput(b)
  if (b.email && audienceContactRows.some(r => r.email === b.email)) throw guestErr(409, { error: 'duplicate' })
  const now = new Date().toISOString()
  const row: AudienceContactRow = {
    id: newId('ac'), name: b.name, email: b.email, phone: b.phone ?? '',
    status: b.status ?? 'active', source: b.source,
    consent_basis: b.consent.basis, consent_recorded_at: b.consent.recorded_at ?? now,
    consent_form_text: b.consent.form_text ?? '', double_opt_in_confirmed_at: b.consent.double_opt_in_confirmed_at ?? null,
    created_at: now, updated_at: now,
  }
  audienceContactRows.push(row)
  return audienceContactView(row)
}

export function updateAudienceContact(id: string, b: AudienceContactInput): AudienceContact {
  const r = audienceContactRows.find(c => c.id === id)
  if (!r) throw guestErr(404, { error: 'not_found' })
  validateAudienceInput({ ...b, consent: b.consent ?? { basis: r.consent_basis, form_text: r.consent_form_text } })
  r.name = b.name
  r.email = b.email
  r.phone = b.phone ?? ''
  r.source = b.source
  r.updated_at = new Date().toISOString()
  return audienceContactView(r)
}

export function setAudienceStatus(id: string, status: AudienceContactStatus) {
  const r = audienceContactRows.find(c => c.id === id)
  if (!r) throw guestErr(404, { error: 'not_found' })
  if (!AUDIENCE_STATUSES.includes(status)) throw invalidField('status', AUDIENCE_STATUSES.join(', '))
  r.status = status
  r.updated_at = new Date().toISOString()
}

export function deleteAudienceContact(id: string) {
  const i = audienceContactRows.findIndex(c => c.id === id)
  if (i < 0) throw guestErr(404, { error: 'not_found' })
  audienceContactRows.splice(i, 1)
}

export function importAudienceCSV(rows: { name?: string, email?: string, phone?: string }[], consent: AudienceConsentInput): AudienceImportResult {
  const result: AudienceImportResult = { added: 0, updated: 0, duplicates: 0, invalid: 0 }
  for (const row of rows) {
    const name = (row.name ?? '').trim()
    const email = (row.email ?? '').trim()
    if (!name && !email) {
      result.invalid++
      continue
    }
    if (email && audienceContactRows.some(r => r.email === email)) {
      result.duplicates++
      continue
    }
    const now = new Date().toISOString()
    audienceContactRows.push({
      id: newId('ac'), name, email, phone: row.phone ?? '',
      status: 'active', source: 'csv',
      consent_basis: consent.basis, consent_recorded_at: consent.recorded_at ?? now,
      consent_form_text: consent.form_text ?? '', double_opt_in_confirmed_at: consent.double_opt_in_confirmed_at ?? null,
      created_at: now, updated_at: now,
    })
    result.added++
  }
  return result
}

function audienceSegmentMatching(filter: AudienceSegmentFilter): number {
  return audienceContactRows.filter((r) => {
    if (filter.status && r.status !== filter.status) return false
    if (filter.source && r.source !== filter.source) return false
    if (filter.since_days) {
      const cutoff = Date.now() - filter.since_days * DAY
      if (Date.parse(r.created_at) < cutoff) return false
    }
    return true
  }).length
}

function audienceSegmentView(s: { id: string, name: string, filter: AudienceSegmentFilter }): AudienceSegment {
  const now = new Date().toISOString()
  return { id: s.id, name: s.name, filter: s.filter, matching: audienceSegmentMatching(s.filter), created_at: now, updated_at: now }
}

export function listAudienceSegments(): AudienceSegment[] {
  return audienceSegmentRows.map(audienceSegmentView)
}

export function createAudienceSegment(b: { name: string, filter: AudienceSegmentFilter }): AudienceSegment {
  const name = (b.name ?? '').trim()
  if (!name) throw invalidField('name', 'required')
  if (audienceSegmentRows.some(s => s.name === name)) throw guestErr(409, { error: 'segment_exists' })
  const row = { id: newId('as'), name, filter: b.filter ?? {} }
  audienceSegmentRows.push(row)
  return audienceSegmentView(row)
}

export function updateAudienceSegment(id: string, b: { name: string, filter: AudienceSegmentFilter }): AudienceSegment {
  const s = audienceSegmentRows.find(x => x.id === id)
  if (!s) throw guestErr(404, { error: 'not_found' })
  const name = (b.name ?? '').trim()
  if (!name) throw invalidField('name', 'required')
  if (audienceSegmentRows.some(x => x.name === name && x.id !== id)) throw guestErr(409, { error: 'segment_exists' })
  s.name = name
  s.filter = b.filter ?? {}
  return audienceSegmentView(s)
}

export function deleteAudienceSegment(id: string) {
  const i = audienceSegmentRows.findIndex(s => s.id === id)
  if (i < 0) throw guestErr(404, { error: 'not_found' })
  audienceSegmentRows.splice(i, 1)
}
