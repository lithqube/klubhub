// Browser-only /api/v1/outreach API. No email is sent: a demo request is
// recorded as sent and delivered in local state, never handed to a provider.
import { uuid } from '../../../shared/finance-mock/rules'
import type { Contact } from '../../types/gig'
import type { AvailabilitySlot, MatchedContact, OutreachRequest, OutreachTemplate, SlotMatch, Suppression } from '../../types/outreach'
import { formatSlotFee, renderRequestText } from '../../utils/outreach'
import { daysFrom } from '../lib/util'
import type { DemoRouter } from '../router'
import { apiError, json, noContent, notFound } from '../router'
import type { DemoContext, DemoRequest, DemoState } from '../types'

interface OutreachData {
  slots: AvailabilitySlot[]
  templates: OutreachTemplate[]
  requests: OutreachRequest[]
  suppressions: Suppression[]
}
type OutreachState = DemoState & { outreach?: OutreachData }
const root = '/api/v1/outreach'
const body = (req: DemoRequest) => (req.body && typeof req.body === 'object' && !(req.body instanceof FormData) ? req.body : {}) as Record<string, unknown>
const validation = (fields: Record<string, string>) => apiError(422, 'validation failed: ' + Object.keys(fields).sort().map(k => `${k}: ${fields[k]}`).join('; '))
const sameCity = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase()
const validId = (id: string) => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id)
const invalidId = (id: string | undefined, name: string) => id && !validId(id) ? apiError(400, `invalid ${name} id`) : null

/** Keep the seed relative to the visitor's date, like seed/gigs.ts. */
export function seedOutreach(now: Date): OutreachData {
  const stamp = daysFrom(now, -3, 12)
  const berlin = daysFrom(now, 14).slice(0, 10)
  const paris = daysFrom(now, 28).slice(0, 10)
  const slots: AvailabilitySlot[] = [
    { id: '00000000-0000-4000-8000-000000000601', date: berlin, city: 'Berlin', country: 'DE', genres: ['house', 'techno'], fee_min: 500, fee_max: 800, currency: 'EUR', notes: 'Available for an evening set.', created_at: stamp },
    { id: '00000000-0000-4000-8000-000000000602', date: paris, city: 'Paris', country: 'FR', genres: ['house'], fee_min: 600, fee_max: null, currency: 'EUR', notes: '', created_at: stamp },
  ]
  const template: OutreachTemplate = {
    id: '00000000-0000-4000-8000-000000000603', name: 'Default booking request', direction: 'dj_to_promoter',
    subject: 'Booking request: {{dj_name}} in {{city}} on {{date}}',
    body: "Hi {{promoter_name}},\n\nI'm {{dj_name}} and I'm available in {{city}} on {{date}}. Would you be interested in booking a set?\n\nGenres: {{genres}}\nFee: {{fee}}\n\nIf you're interested, please reply to {{dj_email}}. If you prefer no further messages, reply stop.\n\nBest,\n{{dj_name}}",
    is_default: true, version: 1, created_at: stamp, updated_at: stamp,
  }
  return {
    slots, templates: [template], suppressions: [
      { id: '00000000-0000-4000-8000-000000000605', email: 'frieda@stahl.example', reason: 'replied stop', created_at: stamp },
      { id: '00000000-0000-4000-8000-000000000606', email: 'stopme@example.com', reason: 'manual', created_at: stamp },
    ],
    requests: [{
      id: '00000000-0000-4000-8000-000000000604', contact_id: '00000000-0000-4000-8000-000000000501',
      availability_slot_id: slots[0]!.id, template_id: template.id, to_email: 'alex@alpha.example',
      subject: `Booking request: Sam Example in Berlin on ${berlin}`,
      body: `Hi Alex Example,\n\nI'm Sam Example and I'm available in Berlin on ${berlin}. Would you be interested in booking a set?`,
      status: 'sent', email_status: 'delivered', sent_at: stamp, created_at: stamp,
    }],
  }
}

// Old v1 localStorage documents predate outreach. Initialize them lazily and
// persist on the first write; existing demo changes remain intact.
function data(c: DemoContext): OutreachData {
  const state = c.state as OutreachState
  return state.outreach ??= seedOutreach(c.now())
}

function validateSlot(b: Record<string, unknown>, c: DemoContext) {
  const city = String(b.city ?? '').trim()
  const country = String(b.country ?? '').trim()
  const date = String(b.date ?? '')
  const genres = Array.isArray(b.genres) ? b.genres.map(x => String(x).trim()) : []
  const fee_min = b.fee_min == null ? null : Number(b.fee_min)
  const fee_max = b.fee_max == null ? null : Number(b.fee_max)
  const currency = b.currency == null ? null : String(b.currency).trim().toUpperCase() || null
  const notes = String(b.notes ?? '').trim()
  if (date && (!/^\d{4}-\d{2}-\d{2}$/.test(date) || new Date(`${date}T00:00:00Z`).toISOString().slice(0, 10) !== date)) return { error: apiError(400, 'invalid JSON body') }
  const errors: Record<string, string> = {}
  if (!city) errors.city = 'required'
  else if (city.length > 80) errors.city = 'exceeds 80 characters'
  if (country.length > 80) errors.country = 'exceeds 80 characters'
  if (!date) errors.date = 'required'
  else if (date < c.now().toISOString().slice(0, 10)) errors.date = 'must not be in the past'
  if (genres.length > 12) errors.genres = 'at most 12 genres'
  else if (genres.some(g => !g)) errors.genres = 'genres must not be empty'
  else if (genres.some(g => g.length > 40)) errors.genres = 'each genre exceeds 40 characters'
  if (fee_min !== null && (!Number.isFinite(fee_min) || fee_min < 0)) errors.fee_min = 'must not be negative'
  if (fee_max !== null && (!Number.isFinite(fee_max) || fee_max < 0)) errors.fee_max = 'must not be negative'
  if (fee_min !== null && fee_max !== null && fee_min > fee_max) errors.fee_min = 'must not exceed fee_max'
  if (currency && !/^[A-Z]{3}$/.test(currency)) errors.currency = 'must be a 3-letter ISO 4217 code'
  if (notes.length > 2000) errors.notes = 'exceeds 2000 characters'
  return Object.keys(errors).length ? { error: validation(errors) } : { value: { date, city, country, genres, fee_min, fee_max, currency, notes } }
}

function templateFields(b: Record<string, unknown>): Record<string, string> {
  const fields: Record<string, string> = {}
  for (const [key, limit] of [['name', 120], ['subject', 200], ['body', 20000]] as const) {
    if (b[key] === undefined) continue
    const value = String(b[key] ?? '').trim()
    if (!value) fields[key] = 'required'
    else if (value.length > limit) fields[key] = `exceeds ${limit} characters`
  }
  return fields
}

function match(c: DemoContext, slot: AvailabilitySlot, suppressions: Suppression[]): SlotMatch {
  const blocked = new Set(suppressions.map(s => s.email))
  const contacts: MatchedContact[] = c.state.contacts.filter(k => k.type === 'promoter' && k.deleted_at === null).flatMap((k: Contact) => {
    const related = c.state.gigs.filter(g => g.city && sameCity(g.city, slot.city) && (
      g.promoter_email.toLowerCase() === k.email.toLowerCase() ||
      c.state.gigLinks[g.id]?.contacts.some(link => link.contact_id === k.id)
    ))
    if (!related.length || (k.email && blocked.has(k.email.trim().toLowerCase()))) return []
    return [{ id: k.id, name: k.name, ...(k.company ? { company: k.company } : {}), email: k.email,
      has_email: !!k.email, city: slot.city, past_gig_in_city: related.some(g => g.status === 'played') }]
  }).sort((a, b) => Number(b.past_gig_in_city) - Number(a.past_gig_in_city) || a.name.localeCompare(b.name))
  const venues = c.state.venues.filter(v => v.deleted_at === null && sameCity(v.city, slot.city))
    .map(v => ({ id: v.id, name: v.name, city: v.city })).sort((a, b) => a.name.localeCompare(b.name))
  return { slot, contacts, venues }
}

export function registerOutreach(r: DemoRouter): void {
  r.on('GET', `${root}/availability/slots`, (_req, c) => json([...data(c).slots].sort((a, b) => a.date.localeCompare(b.date) || a.city.localeCompare(b.city))))
  r.on('POST', `${root}/availability/slots`, (req, c) => {
    const parsed = validateSlot(body(req), c)
    if (parsed.error) return parsed.error
    const slot = { id: uuid(), ...parsed.value!, created_at: c.now().toISOString() }
    const d = data(c)
    if (d.slots.some(s => s.date === slot.date && s.city === slot.city && s.country === slot.country)) return apiError(409, 'conflict')
    d.slots.push(slot)
    return json(slot, 201)
  })
  r.on('DELETE', `${root}/availability/slots/:id`, (req, c) => {
    const bad = invalidId(req.params.id, 'slot')
    if (bad) return bad
    const d = data(c)
    const i = d.slots.findIndex(s => s.id === req.params.id)
    if (i < 0) return notFound()
    d.slots.splice(i, 1)
    for (const row of d.requests) if (row.availability_slot_id === req.params.id) row.availability_slot_id = null
    return noContent()
  })
  r.on('GET', `${root}/availability/matches`, (req, c) => {
    const id = req.query.get('slot_id')
    if (id && !validId(id)) return apiError(400, 'invalid slot_id')
    const d = data(c)
    const slots = id ? d.slots.filter(s => s.id === id) : [...d.slots].sort((a, b) => a.date.localeCompare(b.date))
    if (id && !slots.length) return notFound()
    return json(slots.map(s => match(c, s, d.suppressions)))
  })

  r.on('GET', `${root}/templates`, (_req, c) => json([...data(c).templates].sort((a, b) => Number(b.is_default) - Number(a.is_default) || a.name.localeCompare(b.name))))
  r.on('POST', `${root}/templates`, (req, c) => {
    const b = body(req)
    const errors = templateFields(b)
    for (const key of ['name', 'subject', 'body']) if (b[key] === undefined) errors[key] = 'required'
    if (Object.keys(errors).length) return validation(errors)
    const d = data(c)
    const name = String(b.name).trim()
    if (d.templates.some(t => t.name.toLowerCase() === name.toLowerCase())) return apiError(409, 'conflict')
    const stamp = c.now().toISOString()
    if (b.is_default === true) d.templates.forEach(t => { t.is_default = false })
    const template: OutreachTemplate = { id: uuid(), name, direction: 'dj_to_promoter', subject: String(b.subject).trim(), body: String(b.body).trim(),
      is_default: b.is_default === true, version: 1, created_at: stamp, updated_at: stamp }
    d.templates.push(template)
    return json(template, 201)
  })
  r.on('PUT', `${root}/templates/:id`, (req, c) => {
    const bad = invalidId(req.params.id, 'template')
    if (bad) return bad
    const d = data(c)
    const t = d.templates.find(x => x.id === req.params.id)
    if (!t) return notFound()
    const b = body(req)
    const errors = templateFields(b)
    if (!Number.isInteger(b.version) || Number(b.version) < 1) errors.version = 'required (optimistic concurrency token)'
    if (Object.keys(errors).length) return validation(errors)
    if (b.version !== t.version) return apiError(409, 'conflict')
    const name = b.name === undefined ? t.name : String(b.name).trim()
    if (d.templates.some(x => x.id !== t.id && x.name.toLowerCase() === name.toLowerCase())) return apiError(409, 'conflict')
    if (b.is_default === true) d.templates.forEach(x => { x.is_default = false })
    t.name = name
    if (b.subject !== undefined) t.subject = String(b.subject).trim()
    if (b.body !== undefined) t.body = String(b.body).trim()
    if (b.is_default !== undefined) t.is_default = b.is_default === true
    t.version++
    t.updated_at = c.now().toISOString()
    return json(t)
  })
  r.on('DELETE', `${root}/templates/:id`, (req, c) => {
    const bad = invalidId(req.params.id, 'template')
    if (bad) return bad
    const d = data(c)
    const i = d.templates.findIndex(t => t.id === req.params.id)
    if (i < 0) return notFound()
    d.templates.splice(i, 1)
    for (const row of d.requests) if (row.template_id === req.params.id) row.template_id = null
    return noContent()
  })

  r.on('GET', `${root}/requests`, (_req, c) => json([...data(c).requests].sort((a, b) => b.created_at.localeCompare(a.created_at))))
  r.on('POST', `${root}/requests`, (req, c) => {
    const b = body(req)
    const id = String(b.contact_id ?? '')
    if (!id || !validId(id)) return validation({ contact_id: 'required' })
    const d = data(c)
    const contact = c.state.contacts.find(k => k.id === id && k.type === 'promoter' && k.deleted_at === null)
    if (!contact) return notFound()
    if (!contact.email) return apiError(422, 'contact has no email address')
    if (d.suppressions.some(s => s.email === contact.email.trim().toLowerCase())) return apiError(409, 'address is suppressed')
    const info = c.state.settings.contact_info
    let dj: Record<string, unknown> = {}
    if (typeof info === 'object' && info !== null) dj = info as Record<string, unknown>
    else if (typeof info === 'string' && info.includes('@')) dj = { email: info }
    if (!String(dj.email ?? '').trim()) return apiError(422, 'no DJ contact email configured')
    const templateId = b.template_id == null ? null : String(b.template_id)
    const template = templateId ? d.templates.find(t => t.id === templateId) : d.templates.find(t => t.is_default)
    if (templateId && !template) return notFound()
    const slotId = b.availability_slot_id == null ? null : String(b.availability_slot_id)
    const slot = slotId ? d.slots.find(s => s.id === slotId) : null
    if (slotId && !slot) return notFound()
    const exportId = b.epk_export_id == null ? null : String(b.epk_export_id)
    if (exportId) {
      const exp = c.state.exports.find(e => e.id === exportId)
      if (!exp) return notFound()
      if (!exp.garageObjectKey) return apiError(422, 'epk export has no garage object key')
    }
    const subject = b.subject == null ? template?.subject ?? 'Booking request: {{dj_name}}' : String(b.subject)
    const text = b.body == null ? template?.body ?? 'Hi {{promoter_name}},' : String(b.body)
    const errors: Record<string, string> = {}
    if (subject.length > 200) errors.subject = 'exceeds 200 characters'
    if (text.length > 20000) errors.body = 'exceeds 20000 characters'
    if (Object.keys(errors).length) return validation(errors)
    const today = c.now().toISOString().slice(0, 10)
    if (d.requests.filter(x => x.created_at.slice(0, 10) === today).length >= 50) return apiError(429, 'daily outreach limit reached')
    const vars: Record<string, string> = {
      dj_name: String(dj.name ?? c.state.settings.dj_name ?? ''), dj_email: String(dj.email), dj_phone: String(dj.phone ?? ''),
      promoter_name: contact.name, promoter_company: contact.company ?? '', epk_link: '',
      city: slot?.city ?? '', country: slot?.country ?? '', date: slot?.date ?? '', genres: slot?.genres.join(', ') ?? '',
      fee: slot ? formatSlotFee(slot) : '', notes: slot?.notes ?? '',
    }
    const stamp = c.now().toISOString()
    const row: OutreachRequest = { id: uuid(), contact_id: contact.id, availability_slot_id: slotId, template_id: template?.id ?? null,
      epk_export_id: exportId, to_email: contact.email, subject: renderRequestText(subject, vars).replace(/[\r\n]+/g, ' ').trim(),
      body: renderRequestText(text, vars), status: 'sent', email_status: 'delivered', sent_at: stamp, created_at: stamp }
    d.requests.unshift(row)
    return json(row, 201)
  })
  r.on('GET', `${root}/requests/:id`, (req, c) => {
    const bad = invalidId(req.params.id, 'request')
    if (bad) return bad
    const row = data(c).requests.find(x => x.id === req.params.id)
    return row ? json(row) : notFound()
  })

  r.on('GET', `${root}/suppressions`, (_req, c) => json([...data(c).suppressions].sort((a, b) => b.created_at.localeCompare(a.created_at))))
  r.on('POST', `${root}/suppressions`, (req, c) => {
    const b = body(req)
    const email = String(b.email ?? '').trim().toLowerCase()
    const reason = String(b.reason ?? '').trim()
    const errors: Record<string, string> = {}
    if (!email) errors.email = 'required'
    else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) errors.email = 'must be a valid email address'
    if (reason.length > 500) errors.reason = 'exceeds 500 characters'
    if (Object.keys(errors).length) return validation(errors)
    const d = data(c)
    const existing = d.suppressions.find(s => s.email === email)
    if (existing) return json(existing, 201)
    const row: Suppression = { id: uuid(), email, reason, created_at: c.now().toISOString() }
    d.suppressions.unshift(row)
    return json(row, 201)
  })
  r.on('DELETE', `${root}/suppressions/:id`, (req, c) => {
    const bad = invalidId(req.params.id, 'suppression')
    if (bad) return bad
    const d = data(c)
    const i = d.suppressions.findIndex(s => s.id === req.params.id)
    if (i < 0) return notFound()
    d.suppressions.splice(i, 1)
    return noContent()
  })
}
