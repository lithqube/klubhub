import type { EventInput } from '~/types/event'
import { assertVersion, findEvent, recheck, venues } from '../-mockDb'
export default defineEventHandler(async (event) => {
  const e = findEvent(getRouterParam(event, 'id')!)
  const b = await readBody<{ version: number, event: EventInput }>(event)
  assertVersion(e, b.version)
  const start = Date.parse(b.event.doors_at ?? b.event.starts_at)
  const cut = e.lineup.filter(l => l.set_start && (Date.parse(l.set_start) < start || Date.parse(l.set_end!) > Date.parse(b.event.ends_at)))
  if (cut.length) throw createError({ statusCode: 409, data: { error: 'cuts_sets', entries: cut.map(l => l.id) } })
  const venue = venues.find(v => v.id === b.event.venue_id) ?? null
  Object.assign(e, b.event, { venue: venue ? { id: venue.id, name: venue.name, city: venue.city } : null })
  return recheck(e)
})
