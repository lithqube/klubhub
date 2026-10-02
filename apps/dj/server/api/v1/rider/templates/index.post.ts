import { createTemplate, validateRiderText } from '../_state'

export default defineEventHandler(async (event) => {
  const body = (await readBody(event)) ?? {}
  const invalid = validateRiderText(body, true)
  if (invalid) throw createError({ statusCode: 422, statusMessage: invalid })
  const created = createTemplate(body ?? {})
  setResponseStatus(event, 201)
  return { data: created }
})