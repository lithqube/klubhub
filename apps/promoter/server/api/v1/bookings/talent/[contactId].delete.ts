import { deleteTalent } from '../../-mockDb'
// DELETE /api/v1/bookings/talent/{contactId} (talent.write).
export default defineEventHandler((event) => {
  deleteTalent(getRouterParam(event, 'contactId')!)
  setResponseStatus(event, 204)
  return null
})
