import { deleteAttachment } from '../_state'

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid attachment id' })
  }
  const ok = deleteAttachment(id)
  if (!ok) {
    throw createError({ statusCode: 404, statusMessage: 'attachment not found' })
  }
  setResponseStatus(event, 204)
  return ''
})