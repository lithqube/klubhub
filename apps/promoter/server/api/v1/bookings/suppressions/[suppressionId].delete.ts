import { deleteSuppression } from '../../-mockDb'
// DELETE /api/v1/bookings/suppressions/{suppressionId} (booking.write).
export default defineEventHandler((event) => {
  deleteSuppression(getRouterParam(event, 'suppressionId')!)
  setResponseStatus(event, 204)
  return null
})
