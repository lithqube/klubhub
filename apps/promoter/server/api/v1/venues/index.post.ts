import type { VenueInput } from '~/types/event'
import { newId, venues } from '../-mockDb'
export default defineEventHandler(async (event) => {
  const b = await readBody<VenueInput>(event)
  if (!b?.name?.trim()) throw createError({ statusCode: 422, data: { error: 'invalid', field: 'name', problem: 'required, at most 200 characters' } })
  const v = {
    id: newId('v'), name: b.name.trim(), city: b.city ?? '', country: b.country, timezone: b.timezone || 'UTC', capacity: b.capacity,
    curfew_local: b.curfew_local, tech_notes: b.tech_notes ?? '', archived_at: null,
    rooms: (b.rooms ?? []).map((r, i) => ({ id: newId('r'), name: r.name, capacity: r.capacity, position: i })),
    protected: { address: !!b.protected?.address, geo: !!b.protected?.geo, contact: !!(b.protected?.contact_name || b.protected?.contact_email || b.protected?.contact_phone) },
  }
  venues.push(v)
  setResponseStatus(event, 201)
  return v
})
