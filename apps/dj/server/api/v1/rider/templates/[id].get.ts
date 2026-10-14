import { db } from '../_state'

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid template id' })
  }
  const t = db.getTemplate(id)
  if (!t) {
    throw createError({ statusCode: 404, statusMessage: 'rider record not found' })
  }
  return { data: t }
})
