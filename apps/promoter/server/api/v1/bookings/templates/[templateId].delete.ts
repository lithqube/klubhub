import { deleteTemplate } from '../../-mockDb'
// DELETE /api/v1/bookings/templates/{templateId} (booking.write): soft delete.
export default defineEventHandler((event) => {
  deleteTemplate(getRouterParam(event, 'templateId')!)
  setResponseStatus(event, 204)
  return null
})
