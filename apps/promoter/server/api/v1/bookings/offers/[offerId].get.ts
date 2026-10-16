import { getOffer } from '../../-mockDb'
// GET /api/v1/bookings/offers/{offerId} (booking.read).
export default defineEventHandler((event) => {
  return getOffer(getRouterParam(event, 'offerId')!)
})
