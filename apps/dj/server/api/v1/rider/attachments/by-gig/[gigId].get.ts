import { db } from '../../_state'

export default defineEventHandler((event) => {
  const gigId = getRouterParam(event, 'gigId')
  if (!gigId) {
    throw createError({ statusCode: 400, statusMessage: 'invalid gigId' })
  }
  // Matches real API: missing → 200 {data:null} (NOT 404) so the
  // frontend can render an "no rider yet" state without an error path.
  return { data: db.getAttachmentByGig(gigId) }
})
