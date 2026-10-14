import { db, unwrap } from '../_state'

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid attachment id' })
  }
  unwrap(db.deleteAttachment(id))
  setResponseStatus(event, 204)
  return ''
})
