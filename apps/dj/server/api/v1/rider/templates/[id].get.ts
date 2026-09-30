import { getTemplate } from '../../_state'

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid template id' })
  }
  const t = getTemplate(id)
  if (!t) {
    throw createError({ statusCode: 404, statusMessage: 'template not found' })
  }
  return { data: t }
})