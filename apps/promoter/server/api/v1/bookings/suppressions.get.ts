import { listSuppressions } from '../-mockDb'
// GET /api/v1/bookings/suppressions (booking.read): the do-not-contact list
// (blind index only — no addresses in the response).
export default defineEventHandler(() => listSuppressions())
