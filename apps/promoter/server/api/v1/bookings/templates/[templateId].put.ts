import { updateTemplate } from '../../-mockDb'
// PUT /api/v1/bookings/templates/{templateId} (booking.write): pointer patch
// with optimistic concurrency on version (409 on mismatch).
export default defineEventHandler(async (event) => {
  return updateTemplate(getRouterParam(event, 'templateId')!, await readBody(event))
})
