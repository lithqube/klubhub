import { createAttachment, getAttachmentByGig, getTemplate, validateRiderText } from '../_state'

export default defineEventHandler(async (event) => {
  const body = await readBody(event)
  if (!body?.gigId) {
    throw createError({ statusCode: 400, statusMessage: 'invalid gigId' })
  }
  const invalid = validateRiderText(body, false)
  if (invalid) throw createError({ statusCode: 422, statusMessage: invalid })
  // The real API answers 404 for a template that no longer exists; without
  // this the mock stored the dangling id and created empty sections.
  if (body.templateId && !getTemplate(body.templateId)) {
    throw createError({ statusCode: 404, statusMessage: 'template not found' })
  }
  // Simulate the 409 conflict: if an attachment already exists for this
  // gig, return 409 (matches real API semantics).
  const existing = getAttachmentByGig(body.gigId)
  if (existing) {
    throw createError({ statusCode: 409, statusMessage: 'rider attachment already exists for this gig' })
  }
  const created = createAttachment(body)
  setResponseStatus(event, 201)
  return { data: created }
})