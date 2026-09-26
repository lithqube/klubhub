import type { VenueInput } from '~/types/event'
import { newId, venues } from '../-mockDb'
export default defineEventHandler(async (event) => {
  const v = venues.find(x => x.id === getRouterParam(event, 'id'))
  if (!v) throw createError({ statusCode: 404, data: { error: 'not_found' } })
  const b = await readBody<VenueInput>(event)
  Object.assign(v, {
    name: b.name, city: b.city, country: b.country, timezone: b.timezone, capacity: b.capacity,
    curfew_local: b.curfew_local, tech_notes: b.tech_notes,
    rooms: b.rooms.map((r, i) => ({ id: r.id ?? newId('r'), name: r.name, capacity: r.capacity, position: i })),
  })
  if (b.protected) {
    v.protected = { address: !!b.protected.address, geo: !!b.protected.geo, contact: !!(b.protected.contact_name || b.protected.contact_email || b.protected.contact_phone) }
  }
  return v
})
