import { deleteTemplate } from '../../_state'

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid template id' })
  }
  const ok = deleteTemplate(id)
  if (!ok) {
    throw createError({ statusCode: 404, statusMessage: 'template not found' })
  }
  setResponseStatus(event, 204)
  return ''
})