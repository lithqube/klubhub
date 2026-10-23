import { sendOffer } from '../-mockDb'
// POST /api/v1/bookings/offers (booking.write): render + send one offer, log it.
export default defineEventHandler(async (event) => {
  const r = sendOffer(await readBody(event))
  setResponseStatus(event, 201)
  return r
})
