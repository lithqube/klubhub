import { createAttachment } from '../../_state'

export default defineEventHandler(async (event) => {
  const body = await readBody(event)
  if (!body?.gigId) {
    throw createError({ statusCode: 400, statusMessage: 'invalid gigId' })
  }
  // Simulate the 409 conflict: if an attachment already exists for this
  // gig, return 409 (matches real API semantics).
  const existing = (await import('../../_state')).getAttachmentByGig(body.gigId)
  if (existing) {
    throw createError({ statusCode: 409, statusMessage: 'rider attachment already exists for this gig' })
  }
  const created = createAttachment(body)
  setResponseStatus(event, 201)
  return { data: created }
})