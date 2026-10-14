import { db, unwrap } from '../_state'

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid template id' })
  }
  unwrap(db.deleteTemplate(id))
  setResponseStatus(event, 204)
  return ''
})
