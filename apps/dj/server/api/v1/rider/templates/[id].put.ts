import { updateTemplate } from '../../_state'

export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid template id' })
  }
  const body = await readBody(event)
  const updated = updateTemplate(id, body ?? {})
  if (!updated) {
    throw createError({ statusCode: 404, statusMessage: 'template not found' })
  }
  return { data: updated }
})