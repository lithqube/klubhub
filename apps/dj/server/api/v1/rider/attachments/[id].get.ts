import { getAttachment } from '../../_state'

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid attachment id' })
  }
  const a = getAttachment(id)
  if (!a) {
    throw createError({ statusCode: 404, statusMessage: 'attachment not found' })
  }
  return { data: a }
})