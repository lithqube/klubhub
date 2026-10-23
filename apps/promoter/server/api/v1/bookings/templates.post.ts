import { createTemplate } from '../-mockDb'
// POST /api/v1/bookings/templates (booking.write).
export default defineEventHandler(async (event) => {
  const r = createTemplate(await readBody(event))
  setResponseStatus(event, 201)
  return r
})
