import { matchSlot } from '../../../-mockDb'
// GET /api/v1/bookings/slots/{slotId}/matches (booking.read): ranked talent
// matches for the slot (SPEC §8).
export default defineEventHandler((event) => {
  return matchSlot(getRouterParam(event, 'slotId')!)
})
