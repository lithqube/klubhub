import { deleteSlot } from '../../-mockDb'
// DELETE /api/v1/bookings/slots/{slotId} (booking.write).
export default defineEventHandler((event) => {
  deleteSlot(getRouterParam(event, 'slotId')!)
  setResponseStatus(event, 204)
  return null
})
