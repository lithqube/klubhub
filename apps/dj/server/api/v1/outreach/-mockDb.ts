// Nitro adapter for the in-memory outreach mock (frontend-only dev). The
// rules mirror api/internal/outreach (mounted at /api/v1/outreach): the
// availability slots, city matching, templates, the sent log and the
// do-not-contact list. The leading "-" keeps Nitro from treating this file
// as a route; it lives under server/api/v1 so production builds drop it
// together with the other mocks (scripts/build-prod.mjs).
//
// Unlike the promoter side, the DJ app is single-user OSS: suppressions
// store the email in plaintext (consistent with contacts.email). All names
// below are fictional.

import type { H3Event } from 'h3'
import type {
  AvailabilitySlot,
  MatchedContact,
  MatchedVenue,
  OutreachRequest,
  OutreachTemplate,
  SlotMatch,
  Suppression,
} from '../../../../app/types/outreach'
import { findEpkExport } from '../epk/-state'

export interface MockResult<T = unknown> {
  status: number
  body: T
}

export const ok = <T>(body: T, status = 200): MockResult<T> => ({ status, body })

/** The Go handler writes {"error": "<human text>"} — never codes. */
export const fail = (status: number, message: string): MockResult => ({ status, body: { error: message } })

/** Writes a MockResult to the event: status code + JSON body. */
export function send(event: H3Event, r: MockResult) {
  setResponseStatus(event, r.status)
  return r.body
}

export const idOf = (event: H3Event) => getRouterParam(event, 'id') ?? ''

export async function bodyOf(event: H3Event): Promise<Record<string, unknown>> {
  return ((await readBody(event).catch(() => null)) ?? {}) as Record<string, unknown>
}

// ─── Rows ────────────────────────────────────────────────────────────────────

interface ContactRow {
  id: string
  name: string
  company: string | null
  email: string
  city: string
  past_gig_in_city: boolean
}

interface VenueRow { id: string, name: string, city: string }

interface TemplateRow {
  id: string
  name: string
  direction: 'dj_to_promoter'
  subject: string
  body: string
  is_default: boolean
  version: number
  created_at: string
  updated_at: string
}

interface RequestRow {
  id: string
  contact_id: string
  availability_slot_id: string | null
  template_id: string | null
  epk_export_id: string | null
  email_message_id: string | null
  to_email: string
  subject: string
  body: string
  status: string
  sent_at: string | null
  created_at: string
  email_status?: string
  email_error?: string
}

interface SuppressionRow { id: string, email: string, reason: string, created_at: string }

const iso = (daysAgo: number, hoursAgo = 0): string =>
  new Date(Date.now() - daysAgo * 86400000 - hoursAgo * 3600000).toISOString()

let seq = 0
const newId = (prefix: string) =>
  `${prefix}-${Date.now().toString(36)}-${(seq++).toString(36)}${Math.random().toString(36).slice(2, 6)}`

/** The DJ the requests are sent from (user_settings.contact_info). Mutable
 * so tests can exercise the 422 "no DJ contact email configured" path. */
let djContactInfo = { name: 'SAMPLE_RATE', email: 'samplerate@dj.example', phone: '+49 30 555 0100' }

/** Mirrors the transactional email provider being configured (503 when false). */
let emailAvailable = true

export function setDjContactInfo(info: Partial<typeof djContactInfo>): void {
  djContactInfo = { ...djContactInfo, ...info }
}

export function setEmailAvailable(available: boolean): void {
  emailAvailable = available
}

/** The built-in template seeded by migration 034 (SPEC §7). */
const DEFAULT_TEMPLATE_SUBJECT = 'Booking request: {{dj_name}} in {{city}} on {{date}}'
const DEFAULT_TEMPLATE_BODY = [
  'Hi {{promoter_name}},',
  '',
  "I'm {{dj_name}}, a DJ playing {{genres}}. I'm available in {{city}} on {{date}} and would love to play for you.",
  '',
  'Date: {{date}}',
  'City: {{city}}',
  'Genres: {{genres}}',
  'Fee: {{fee}}',
  '{{notes}}',
  '',
  'My press kit (EPK) is attached to this email.',
  '{{epk_link}}',
  '',
  "If you're interested, just reply to this email — it lands directly in my inbox ({{dj_email}}).",
  '',
  'If you\'d rather not hear from us, reply "stop" and we\'ll remove you.',
  '',
  'Best,',
  '{{dj_name}}',
  '{{dj_phone}}',
].join('\n')

/** The public EPK URL (EPK_PUBLIC_URL) — fills {{epk_link}} at send. */
const EPK_PUBLIC_URL = 'https://klubhub.example/epk/sample-rate'

/** OUTREACH_MAX_PER_DAY (default 50). */
const OUTREACH_MAX_PER_DAY = 50

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/

// The mock member's promoter contacts (contacts.type = 'promoter' — the only
// kind the DJ-side matcher returns).
let contactRows: ContactRow[] = []
let venueRows: VenueRow[] = []
let slotRows: AvailabilitySlot[] = []
let templateRows: TemplateRow[] = []
let requestRows: RequestRow[] = []
let suppressionRows: SuppressionRow[] = []

function seed(): void {
  contactRows = [
    { id: 'pc-anna', name: 'Anna Keller', company: 'Klubnacht Berlin', email: 'anna@klubnacht.example', city: 'Berlin', past_gig_in_city: true },
    { id: 'pc-ben', name: 'Ben Okafor', company: 'Warehouse Nights', email: 'ben@warehouse.example', city: 'Berlin', past_gig_in_city: false },
    { id: 'pc-cara', name: 'Cara Lind', company: null, email: 'cara@lind.example', city: 'Berlin', past_gig_in_city: false },
    // No email on file: matchable, never sendable (OUTR-05 v2.1).
    { id: 'pc-dirk', name: 'Dirk Stein', company: 'Silent Bookings', email: '', city: 'Berlin', past_gig_in_city: false },
    // Hamburg: never matches the Berlin slots (match is by slot city).
    { id: 'pc-emil', name: 'Emil Roth', company: 'Elbe Bookings', email: 'emil@elbe.example', city: 'Hamburg', past_gig_in_city: false },
    // Suppressed: leaves the matchable pool and refuses sends (409).
    { id: 'pc-frieda', name: 'Frieda Stahl', company: 'Stahl Agency', email: 'frieda@stahl.example', city: 'Berlin', past_gig_in_city: false },
  ]
  venueRows = [
    { id: 'v-klub', name: 'Klubnacht', city: 'Berlin' },
    { id: 'v-warehouse', name: 'Warehouse 7', city: 'Berlin' },
    { id: 'v-elbe', name: 'Elbe Stage', city: 'Hamburg' },
  ]
  // Future dates: the date-not-in-the-past rule must never stale the seed.
  slotRows = [
    { id: 'as-berlin', date: '2027-01-16', city: 'Berlin', country: 'DE', genres: ['techno', 'house'], fee_min: 600, fee_max: 900, currency: 'EUR', notes: 'Saturday, main room.', created_at: iso(3) },
    { id: 'as-hamburg', date: '2027-02-20', city: 'Hamburg', country: 'DE', genres: ['techno'], fee_min: 500, fee_max: 500, currency: 'EUR', notes: '', created_at: iso(1) },
  ]
  templateRows = [
    { id: 'tpl-default', name: 'Default booking request', direction: 'dj_to_promoter', subject: DEFAULT_TEMPLATE_SUBJECT, body: DEFAULT_TEMPLATE_BODY, is_default: true, version: 1, created_at: iso(5), updated_at: iso(5) },
    { id: 'tpl-weekend', name: 'Weekend ask', direction: 'dj_to_promoter', subject: 'Weekend slot: {{city}} on {{date}}', body: 'Hi {{promoter_name}},\n\nI have {{city}} open on {{date}} ({{genres}}). Fee: {{fee}}. Interested?\n\n{{dj_name}}', is_default: false, version: 2, created_at: iso(3), updated_at: iso(1) },
  ]
  requestRows = [
    { id: 'or-anna', contact_id: 'pc-anna', availability_slot_id: 'as-berlin', template_id: 'tpl-default', epk_export_id: 'epk-1', email_message_id: 'plunk-mock-1', to_email: 'anna@klubnacht.example', subject: 'Booking request: SAMPLE_RATE in Berlin on 2027-01-16', body: '(rendered request — see the sent log row)', status: 'sent', sent_at: iso(1, 3), created_at: iso(1, 3), email_status: 'delivered' },
    { id: 'or-ben', contact_id: 'pc-ben', availability_slot_id: null, template_id: null, epk_export_id: null, email_message_id: null, to_email: 'ben@warehouse.example', subject: 'Booking request: SAMPLE_RATE in Berlin on 2027-01-16', body: '(rendered request — delivery failed)', status: 'failed', sent_at: null, created_at: iso(2), email_status: 'failed', email_error: 'mailbox full' },
  ]
  suppressionRows = [
    { id: 'sup-1', email: 'frieda@stahl.example', reason: 'replied "stop"', created_at: iso(5) },
    { id: 'sup-2', email: 'stopme@example.com', reason: 'manual', created_at: iso(4) },
  ]
  djContactInfo = { name: 'SAMPLE_RATE', email: 'samplerate@dj.example', phone: '+49 30 555 0100' }
  emailAvailable = true
}

seed()

/** Restores the seeded state (tests mutate the rows and the toggles). */
export function resetOutreachMock(): void {
  seed()
}

// ─── Validation (mirrors validate.go) ────────────────────────────────────────

/** The Go ValidationError text: "validation failed: field: reason; …" (sorted). */
function validationError(fields: Record<string, string>): MockResult {
  const text = 'validation failed: ' + Object.keys(fields).sort().map(f => `${f}: ${fields[f]}`).join('; ')
  return fail(422, text)
}

interface SlotInput {
  date: string
  city: string
  country: string
  genres: string[]
  fee_min: number | null
  fee_max: number | null
  currency: string | null
  notes: string
}

/** Normalizes into `out` and returns the 422 result when invalid. */
function validateSlot(b: Record<string, unknown>, out: SlotInput): MockResult | null {
  const errs: Record<string, string> = {}
  out.city = String(b.city ?? '').trim()
  if (!out.city) errs.city = 'required'
  else if (out.city.length > 80) errs.city = 'exceeds 80 characters'
  out.country = String(b.country ?? '').trim()
  if (out.country.length > 80) errs.country = 'exceeds 80 characters'
  out.date = String(b.date ?? '').trim()
  if (!out.date) {
    errs.date = 'required'
  } else if (!DATE_RE.test(out.date) || Number.isNaN(Date.parse(`${out.date}T00:00:00Z`))) {
    // The Go Date type fails JSON decoding on a malformed date (400).
    return fail(400, 'invalid JSON body')
  } else {
    // Calendar-day comparison against today (local), like the Go validator.
    const now = new Date()
    const todayUTC = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate())
    if (Date.parse(`${out.date}T00:00:00Z`) < todayUTC) errs.date = 'must not be in the past'
  }
  const rawGenres = Array.isArray(b.genres) ? b.genres : []
  if (rawGenres.length > 12) errs.genres = 'at most 12 genres'
  out.genres = []
  for (const g of rawGenres) {
    const t = String(g ?? '').trim()
    if (!t) { errs.genres = 'genres must not be empty'; break }
    if (t.length > 40) { errs.genres = 'each genre exceeds 40 characters'; break }
    out.genres.push(t)
  }
  const num = (v: unknown): number | null => {
    if (v == null || v === '') return null
    const n = Number(v)
    return Number.isFinite(n) ? n : null
  }
  out.fee_min = num(b.fee_min)
  out.fee_max = num(b.fee_max)
  if (out.fee_min != null && out.fee_min < 0) errs.fee_min = 'must not be negative'
  if (out.fee_max != null && out.fee_max < 0) errs.fee_max = 'must not be negative'
  if (out.fee_min != null && out.fee_max != null && out.fee_min > out.fee_max) errs.fee_min = 'must not exceed fee_max'
  out.currency = String(b.currency ?? '').trim().toUpperCase() || null
  if (out.currency && !/^[A-Z]{3}$/.test(out.currency)) errs.currency = 'must be a 3-letter ISO 4217 code'
  out.notes = String(b.notes ?? '').trim()
  if (out.notes.length > 2000) errs.notes = 'exceeds 2000 characters'
  return Object.keys(errs).length ? validationError(errs) : null
}

function validateTemplateFields(b: Record<string, unknown>, out: { name: string, subject: string, body: string }): MockResult | null {
  const errs: Record<string, string> = {}
  out.name = String(b.name ?? '').trim()
  if (!out.name) errs.name = 'required'
  else if (out.name.length > 120) errs.name = 'exceeds 120 characters'
  out.subject = String(b.subject ?? '').trim()
  if (!out.subject) errs.subject = 'required'
  else if (out.subject.length > 200) errs.subject = 'exceeds 200 characters'
  out.body = String(b.body ?? '').trim()
  if (!out.body) errs.body = 'required'
  else if (out.body.length > 20000) errs.body = 'exceeds 20000 characters'
  return Object.keys(errs).length ? validationError(errs) : null
}

// ─── Availability slots ──────────────────────────────────────────────────────

/** ORDER BY date ASC, city ASC. */
export function listSlots(): AvailabilitySlot[] {
  return [...slotRows]
    .sort((a, b) => a.date.localeCompare(b.date) || a.city.localeCompare(b.city))
    .map(s => ({ ...s, genres: [...s.genres] }))
}

export function createSlot(b: Record<string, unknown>): MockResult {
  const input: SlotInput = { date: '', city: '', country: '', genres: [], fee_min: null, fee_max: null, currency: null, notes: '' }
  const bad = validateSlot(b, input)
  if (bad) return bad
  // UNIQUE (date, city, country) — one slot per date/city/country (OUTR-09).
  if (slotRows.some(s => s.date === input.date && s.city === input.city && s.country === input.country)) {
    return fail(409, 'conflict')
  }
  const row: AvailabilitySlot = { id: newId('as'), ...input, created_at: new Date().toISOString() }
  slotRows.push(row)
  return ok({ ...row, genres: [...row.genres] }, 201)
}

export function deleteSlot(id: string): MockResult {
  const i = slotRows.findIndex(s => s.id === id)
  if (i < 0) return fail(404, 'not found')
  slotRows.splice(i, 1)
  // Sent-log rows keep their history (ON DELETE SET NULL).
  for (const r of requestRows) if (r.availability_slot_id === id) r.availability_slot_id = null
  return { status: 204, body: null }
}

// ─── Matches (SPEC §8) ───────────────────────────────────────────────────────

/** Contacts in the slot's city, suppressed excluded, no-email contacts
 * included (has_email=false); ORDER BY past_gig_in_city DESC, name ASC. */
function matchContacts(city: string): MatchedContact[] {
  const c = city.trim().toLowerCase()
  const suppressed = new Set(suppressionRows.map(s => s.email.toLowerCase()))
  return contactRows
    .filter(r => r.city.trim().toLowerCase() === c
      && (r.email === '' || !suppressed.has(r.email.toLowerCase())))
    .map(r => ({
      id: r.id,
      name: r.name,
      ...(r.company ? { company: r.company } : {}),
      email: r.email,
      has_email: r.email !== '',
      city: r.city,
      past_gig_in_city: r.past_gig_in_city,
    }))
    .sort((a, b) => Number(b.past_gig_in_city) - Number(a.past_gig_in_city) || a.name.localeCompare(b.name))
}

/** Venues in the slot's city, ORDER BY name ASC. */
function matchVenues(city: string): MatchedVenue[] {
  const c = city.trim().toLowerCase()
  return venueRows
    .filter(r => r.city.trim().toLowerCase() === c)
    .map(r => ({ ...r }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

export function matchSlots(slotId?: string): MockResult {
  let slots: AvailabilitySlot[]
  if (slotId) {
    const slot = slotRows.find(s => s.id === slotId)
    if (!slot) return fail(404, 'not found')
    slots = [slot]
  } else {
    slots = listSlots()
  }
  const out: SlotMatch[] = slots.map(slot => ({
    slot: { ...slot, genres: [...slot.genres] },
    contacts: matchContacts(slot.city),
    venues: matchVenues(slot.city),
  }))
  return ok(out)
}

// ─── Templates ───────────────────────────────────────────────────────────────

/** ORDER BY is_default DESC, name ASC. */
export function listTemplates(): OutreachTemplate[] {
  return [...templateRows]
    .sort((a, b) => Number(b.is_default) - Number(a.is_default) || a.name.localeCompare(b.name))
    .map(t => ({ ...t }))
}

export function createTemplate(b: Record<string, unknown>): MockResult {
  const input = { name: '', subject: '', body: '' }
  const bad = validateTemplateFields(b, input)
  if (bad) return bad
  // UNIQUE (lower(name)) — one template per name.
  if (templateRows.some(t => t.name.toLowerCase() === input.name.toLowerCase())) {
    return fail(409, 'conflict')
  }
  const now = new Date().toISOString()
  const isDefault = b.is_default === true
  if (isDefault) templateRows.forEach(t => { t.is_default = false })
  const row: TemplateRow = {
    id: newId('tpl'), name: input.name, direction: 'dj_to_promoter',
    subject: input.subject, body: input.body, is_default: isDefault,
    version: 1, created_at: now, updated_at: now,
  }
  templateRows.push(row)
  return ok({ ...row }, 201)
}

export function updateTemplate(id: string, b: Record<string, unknown>): MockResult {
  const row = templateRows.find(t => t.id === id)
  if (!row) return fail(404, 'not found')
  const version = Number(b.version)
  if (!Number.isInteger(version) || version < 1) {
    return validationError({ version: 'required (optimistic concurrency token)' })
  }
  if (row.version !== version) return fail(409, 'conflict')
  if (b.name !== undefined) {
    const name = String(b.name).trim()
    if (!name) return validationError({ name: 'required' })
    if (name.length > 120) return validationError({ name: 'exceeds 120 characters' })
    if (templateRows.some(t => t.id !== id && t.name.toLowerCase() === name.toLowerCase())) {
      return fail(409, 'conflict')
    }
    row.name = name
  }
  if (b.subject !== undefined) {
    const subject = String(b.subject).trim()
    if (!subject) return validationError({ subject: 'required' })
    if (subject.length > 200) return validationError({ subject: 'exceeds 200 characters' })
    row.subject = subject
  }
  if (b.body !== undefined) {
    const body = String(b.body).trim()
    if (!body) return validationError({ body: 'required' })
    if (body.length > 20000) return validationError({ body: 'exceeds 20000 characters' })
    row.body = body
  }
  if (b.is_default !== undefined) {
    if (b.is_default === true) templateRows.forEach(t => { if (t.id !== id) t.is_default = false })
    row.is_default = b.is_default === true
  }
  row.version += 1
  row.updated_at = new Date().toISOString()
  return ok({ ...row })
}

export function deleteTemplate(id: string): MockResult {
  const i = templateRows.findIndex(t => t.id === id)
  if (i < 0) return fail(404, 'not found')
  templateRows.splice(i, 1)
  // Sent-log rows keep their history (ON DELETE SET NULL).
  for (const r of requestRows) if (r.template_id === id) r.template_id = null
  return { status: 204, body: null }
}

// ─── Send (mirrors service.go's SendRequest order) ───────────────────────────

/** formatMoney: two decimals, trailing zeros and the dot trimmed. */
function formatMoney(v: number): string {
  return v.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')
}

/** formatFee: "500 EUR", "500–800 EUR", "from 500 EUR", "up to 800 EUR", "". */
function formatFee(slot: AvailabilitySlot): string {
  const cur = slot.currency ? ` ${slot.currency}` : ''
  const min = slot.fee_min ?? null
  const max = slot.fee_max ?? null
  if (min != null && max != null && min === max) return `${formatMoney(min)}${cur}`
  if (min != null && max != null) return `${formatMoney(min)}–${formatMoney(max)}${cur}`
  if (min != null) return `from ${formatMoney(min)}${cur}`
  if (max != null) return `up to ${formatMoney(max)}${cur}`
  return ''
}

/** Tiny {{var}} substitution; unknown variables render empty (email.Render). */
function render(text: string, vars: Record<string, string>): string {
  return text.replace(/\{\{\s*(\w+)\s*\}\}/g, (_m, name: string) => vars[name] ?? '')
}

function variables(contact: ContactRow, slot: AvailabilitySlot | null): Record<string, string> {
  const vars: Record<string, string> = {
    dj_name: djContactInfo.name,
    dj_email: djContactInfo.email,
    dj_phone: djContactInfo.phone,
    promoter_name: contact.name,
    promoter_company: contact.company ?? '',
    epk_link: EPK_PUBLIC_URL,
  }
  if (slot) {
    vars.city = slot.city
    vars.country = slot.country
    vars.date = slot.date
    vars.genres = slot.genres.join(', ')
    vars.fee = formatFee(slot)
    vars.notes = slot.notes
  }
  return vars
}

/**
 * Sends one booking request: emailAvailable (503) → validate (422) → contact
 * exists (404) → has email (422) → not suppressed (409) → DJ reply-to (422)
 * → template + slot resolve (404) → render → daily cap (429) → EPK pointer
 * (404/422) → insert the row → 201. The mock validates the EPK export BEFORE
 * inserting the request row (the Go flow inserts-then-fails — an internal
 * wart; the mock refuses cleanly with the same 422/404) and always delivers.
 */
export function sendRequest(b: Record<string, unknown>): MockResult {
  if (!emailAvailable) return fail(503, 'email is not available')
  const errs: Record<string, string> = {}
  const contactId = String(b.contact_id ?? '')
  if (!contactId) errs.contact_id = 'required'
  const subjectOverride = b.subject == null ? null : String(b.subject)
  const bodyOverride = b.body == null ? null : String(b.body)
  if (subjectOverride != null && subjectOverride.length > 200) errs.subject = 'exceeds 200 characters'
  if (bodyOverride != null && bodyOverride.length > 20000) errs.body = 'exceeds 20000 characters'
  if (Object.keys(errs).length) return validationError(errs)

  const contact = contactRows.find(r => r.id === contactId)
  if (!contact) return fail(404, 'not found')
  if (!contact.email) return fail(422, 'contact has no email address')
  if (suppressionRows.some(s => s.email.toLowerCase() === contact.email.toLowerCase())) {
    return fail(409, 'address is suppressed')
  }
  // ReplyTo = the DJ's own contact email (OUTR-06 v2.1).
  if (!djContactInfo.email) return fail(422, 'no DJ contact email configured')

  let tpl: TemplateRow
  const templateId = b.template_id == null || b.template_id === '' ? null : String(b.template_id)
  if (templateId) {
    const found = templateRows.find(t => t.id === templateId)
    if (!found) return fail(404, 'not found')
    tpl = found
  } else {
    const def = templateRows.find(t => t.is_default)
    if (!def) return fail(404, 'not found')
    tpl = def
  }

  let slot: AvailabilitySlot | null = null
  const slotId = b.availability_slot_id == null || b.availability_slot_id === '' ? null : String(b.availability_slot_id)
  if (slotId) {
    const found = slotRows.find(s => s.id === slotId)
    if (!found) return fail(404, 'not found')
    slot = found
  }

  const vars = variables(contact, slot)
  const subject = render(subjectOverride ?? tpl.subject, vars)
  const body = render(bodyOverride ?? tpl.body, vars)

  // The window is local midnight in the sender's timezone.
  const today = new Date().toDateString()
  if (requestRows.filter(r => new Date(r.created_at).toDateString() === today).length >= OUTREACH_MAX_PER_DAY) {
    return fail(429, 'daily outreach limit reached')
  }

  const epkExportId = b.epk_export_id == null || b.epk_export_id === '' ? null : String(b.epk_export_id)
  if (epkExportId) {
    const exp = findEpkExport(epkExportId)
    if (!exp) return fail(404, 'not found')
    // A legacy export with an empty garage_object_key (pre-025 minio_path
    // rows) is refused.
    if (!exp.garageObjectKey) return fail(422, 'epk export has no garage object key')
  }

  const now = new Date().toISOString()
  const row: RequestRow = {
    id: newId('or'),
    contact_id: contact.id,
    availability_slot_id: slotId,
    template_id: tpl.id,
    epk_export_id: epkExportId,
    email_message_id: `plunk-mock-${Math.random().toString(36).slice(2, 10)}`,
    to_email: contact.email,
    subject,
    body,
    status: 'sent',
    sent_at: now,
    created_at: now,
    email_status: 'delivered',
  }
  requestRows.unshift(row)
  return ok(requestView(row), 201)
}

// ─── Requests (sent log) ─────────────────────────────────────────────────────

/** The Go view omits null optional keys (omitempty); the app type allows
 * `| null`, so the mock returns them explicitly. */
function requestView(r: RequestRow): OutreachRequest {
  return {
    id: r.id,
    contact_id: r.contact_id,
    availability_slot_id: r.availability_slot_id,
    template_id: r.template_id,
    epk_export_id: r.epk_export_id,
    email_message_id: r.email_message_id,
    to_email: r.to_email,
    subject: r.subject,
    body: r.body,
    status: r.status as OutreachRequest['status'],
    sent_at: r.sent_at,
    created_at: r.created_at,
    email_status: r.email_status,
    email_error: r.email_error,
  }
}

/** ORDER BY created_at DESC (newest first). */
export function listRequests(): OutreachRequest[] {
  return [...requestRows]
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))
    .map(requestView)
}

export function getRequest(id: string): MockResult {
  const row = requestRows.find(r => r.id === id)
  if (!row) return fail(404, 'not found')
  return ok(requestView(row))
}

// ─── Suppressions (do-not-contact list) ──────────────────────────────────────

/** ORDER BY created_at DESC (newest first). */
export function listSuppressions(): Suppression[] {
  return [...suppressionRows]
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))
    .map(s => ({ ...s }))
}

export function addSuppression(b: Record<string, unknown>): MockResult {
  const errs: Record<string, string> = {}
  const email = String(b.email ?? '').trim().toLowerCase()
  if (!email) errs.email = 'required'
  else if (!EMAIL_RE.test(email)) errs.email = 'must be a valid email address'
  const reason = String(b.reason ?? '').trim()
  if (reason.length > 500) errs.reason = 'exceeds 500 characters'
  if (Object.keys(errs).length) return validationError(errs)
  // UNIQUE (email) + ON CONFLICT DO NOTHING: adding an existing address
  // returns the existing row (idempotent).
  const existing = suppressionRows.find(s => s.email === email)
  if (existing) return ok({ ...existing }, 201)
  const row: SuppressionRow = { id: newId('sup'), email, reason, created_at: new Date().toISOString() }
  suppressionRows.unshift(row)
  return ok({ ...row }, 201)
}

export function deleteSuppression(id: string): MockResult {
  const i = suppressionRows.findIndex(s => s.id === id)
  if (i < 0) return fail(404, 'not found')
  suppressionRows.splice(i, 1)
  return { status: 204, body: null }
}
