import { createSlot } from '../../../-mockDb'
// POST /api/v1/bookings/events/{eventId}/slots (booking.write).
export default defineEventHandler(async (event) => {
  const r = createSlot(getRouterParam(event, 'eventId')!, await readBody(event))
  setResponseStatus(event, 201)
  return r
})
