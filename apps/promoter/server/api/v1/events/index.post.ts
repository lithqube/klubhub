import type { EventInput } from '~/types/event'
import { events, newId, recheck, venues } from '../-mockDb'
export default defineEventHandler(async (event) => {
  const b = await readBody<EventInput>(event)
  if (!b?.title?.trim()) throw createError({ statusCode: 422, data: { error: 'invalid', field: 'title', problem: 'required, at most 200 characters' } })
  if (!(Date.parse(b.ends_at) > Date.parse(b.starts_at))) throw createError({ statusCode: 422, data: { error: 'invalid', field: 'ends_at', problem: 'must be after the start' } })
  const venue = venues.find(v => v.id === b.venue_id) ?? null
  const slug = b.title.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
  const e = recheck({
    ...b, id: newId('e'), slug: events.some(x => x.slug === slug) ? `${slug}-2` : slug, status: 'draft', version: 0,
    city: b.city || venue?.city || '', created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
    venue: venue ? { id: venue.id, name: venue.name, city: venue.city } : null,
    stages: (venue?.rooms ?? []).map(r => ({ id: newId('s'), name: r.name, position: r.position, curfew_at: null, changeover_minutes: 0 })),
    lineup: [], issues: [],
  })
  events.push(e)
  setResponseStatus(event, 201)
  return e
})
