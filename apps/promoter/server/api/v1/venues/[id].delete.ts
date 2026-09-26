import { events, venues } from '../-mockDb'
export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  const v = venues.find(x => x.id === id)
  if (!v) throw createError({ statusCode: 404, data: { error: 'not_found' } })
  if (events.some(e => e.venue_id === id && Date.parse(e.ends_at) > Date.now() && e.status !== 'cancelled')) {
    throw createError({ statusCode: 409, data: { error: 'venue_in_use' } })
  }
  v.archived_at = new Date().toISOString()
  setResponseStatus(event, 204)
  return null
})
