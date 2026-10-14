import { db, unwrap } from '../_state'

export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid attachment id' })
  }
  const { updatedAt, ...patch } = (await readBody(event)) ?? {}
  return { data: unwrap(db.updateAttachment(id, patch, updatedAt)) }
})
