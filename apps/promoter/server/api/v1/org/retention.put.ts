import { mockAuthAt, setRetention } from '../-mockDb'

// Mirrors PUT /api/v1/org/retention: {retention_days} 1..365, plus
// {confirm_purge: N} when a shorter period erases N ended events at once
// (409 retention_would_purge otherwise; that also needs a recent sign-in).
export default defineEventHandler(async event => setRetention(await readBody(event), mockAuthAt(event)))
