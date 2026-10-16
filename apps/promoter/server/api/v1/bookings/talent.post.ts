import { createTalent } from '../-mockDb'
// POST /api/v1/bookings/talent (talent.write).
export default defineEventHandler(async (event) => {
  const r = createTalent(await readBody(event))
  setResponseStatus(event, 201)
  return r
})
