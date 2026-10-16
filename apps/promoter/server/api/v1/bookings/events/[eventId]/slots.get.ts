import { listSlots } from '../../../-mockDb'
// GET /api/v1/bookings/events/{eventId}/slots (booking.read).
export default defineEventHandler((event) => {
  return listSlots(getRouterParam(event, 'eventId')!)
})
