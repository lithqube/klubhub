import { createTemplate } from '../_state'

export default defineEventHandler(async (event) => {
  const body = await readBody(event)
  const created = createTemplate(body ?? {})
  setResponseStatus(event, 201)
  return { data: created }
})