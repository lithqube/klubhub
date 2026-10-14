import { db } from '../_state'

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid attachment id' })
  }
  const a = db.getAttachment(id)
  if (!a) {
    throw createError({ statusCode: 404, statusMessage: 'rider record not found' })
  }
  return { data: a }
})
