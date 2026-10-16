import { listTemplates } from '../-mockDb'
// GET /api/v1/bookings/templates (booking.read).
export default defineEventHandler(() => listTemplates())
