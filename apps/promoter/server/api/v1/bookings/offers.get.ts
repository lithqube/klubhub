import { listOffers } from '../-mockDb'
// GET /api/v1/bookings/offers (booking.read): the sent log, newest first.
export default defineEventHandler(() => listOffers())
