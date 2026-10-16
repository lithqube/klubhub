import { listTalent } from '../-mockDb'
// GET /api/v1/bookings/talent?city&genre&q (talent.read).
export default defineEventHandler((event) => {
  const q = getQuery(event)
  return listTalent({
    city: q.city ? String(q.city) : undefined,
    genre: q.genre ? String(q.genre) : undefined,
    q: q.q ? String(q.q) : undefined,
  })
})
