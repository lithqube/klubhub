import { updateTalent } from '../../-mockDb'
// PUT /api/v1/bookings/talent/{contactId} (talent.write).
export default defineEventHandler(async (event) => {
  return updateTalent(getRouterParam(event, 'contactId')!, await readBody(event))
})
