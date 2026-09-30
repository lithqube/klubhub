import { updateAttachment } from '../../_state'

export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid attachment id' })
  }
  const body = await readBody(event)
  const updated = updateAttachment(id, body ?? {})
  if (!updated) {
    throw createError({ statusCode: 404, statusMessage: 'attachment not found' })
  }
  return { data: updated }
})