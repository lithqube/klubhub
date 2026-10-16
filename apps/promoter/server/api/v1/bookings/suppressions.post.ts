import { addSuppression } from '../-mockDb'
// POST /api/v1/bookings/suppressions (booking.write): one-click opt-out.
export default defineEventHandler(async (event) => {
  const r = addSuppression(await readBody(event))
  setResponseStatus(event, 201)
  return r
})
