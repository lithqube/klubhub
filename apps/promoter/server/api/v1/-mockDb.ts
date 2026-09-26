// In-memory data for frontend-only development (no NUXT_PUBLIC_API_BASE).
// Shapes mirror the Go API; timetable issues come from the shared client
// validator so the UI behaves as it will against the server.
import type { EventDetail, EventSummary, Venue } from '~/types/event'
import type {
  AddGuestsInput, AddResult, Allocation, AllocationInput, BulkResult, BulkStatusInput, EntryTerms, Guest, GuestInput, GuestList, GuestPage,
  GuestStatus, ListInput, OverviewRow, StandingList,
} from '~/types/guest'
import type { Organization, OrgProfile } from '~/types/org'
import {
  allocationState, countByStatus, cutoffInstant, fold, headsHeld, holdsQuota, LIST_TYPES, STATUSES,
} from '~/utils/guests'
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

/** Mock session: an owner who turns on TOTP via ACCOUNT → SECURITY. */
export const mockSession = { mfa: false }

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
type GuestRow = Guest & { event_id: string }

const terms = (t: Partial<EntryTerms> = {}): EntryTerms => ({ price_mode: 'free', reduced_price_text: '', perks: [], ...t })

export const guestLists: ListRow[] = [
  { id: 'gl-artist', event_id: 'e-klubnacht', name: 'Artist guests', type: 'artist', collect_contact: false, standing_template_id: 'sl-residents', position: 0, entry_terms: terms({ cutoff_at: iso(6, 2), perks: ['drink token'] }) },
  { id: 'gl-comp', event_id: 'e-klubnacht', name: 'Comp', type: 'comp', collect_contact: false, standing_template_id: null, position: 1, entry_terms: terms() },
  { id: 'gl-industry', event_id: 'e-klubnacht', name: 'Industry', type: 'industry', collect_contact: true, standing_template_id: null, position: 2, entry_terms: terms({ price_mode: 'reduced', reduced_price_text: '€10 before 01:00', cutoff_at: iso(6, 2) }) },
  { id: 'gl-wh-comp', event_id: 'e-warehouse', name: 'Comp', type: 'comp', collect_contact: false, standing_template_id: null, position: 0, entry_terms: terms() },
]

export const allocations: AllocRow[] = [
  { id: 'al-ben', event_id: 'e-klubnacht', list_id: 'gl-artist', label: 'Ben Klock', submitter_contact: 'tour@agency.example', quota: 6, plus_n_max: 1, deadline: iso(6, -3), requires_approval: false, revoked_at: null },
  { id: 'al-dasha', event_id: 'e-klubnacht', list_id: 'gl-artist', label: 'Dasha Rush', submitter_contact: '', quota: 4, plus_n_max: 1, deadline: null, requires_approval: true, revoked_at: null },
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
  return { added: added.map(strip), duplicates }
}

export function updateGuest(eventId: string, id: string, b: GuestInput): Guest {
  const g = guests.find(x => x.id === id && x.event_id === eventId)
  if (!g) throw guestErr(404, { error: 'not_found' })
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
  return strip(g)
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

export function guestPage(eventId: string, filter: { status?: string, list_id?: string }): GuestPage {
  findEvent(eventId)
  if (filter.status && !STATUSES.includes(filter.status as GuestStatus)) throw invalidField('status', STATUSES.join(', '))
  const inList = guests.filter(g => g.event_id === eventId && (!filter.list_id || g.list_id === filter.list_id))
  return { guests: inList.filter(g => !filter.status || g.status === filter.status).map(strip), counts: countByStatus(inList) }
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
      }
    })
}
