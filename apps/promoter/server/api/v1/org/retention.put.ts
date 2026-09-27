import { setRetention } from '../-mockDb'

// Mirrors PUT /api/v1/org/retention: {retention_days} 1..365; recomputes unpurged dates.
export default defineEventHandler(async event => setRetention(await readBody(event)))
