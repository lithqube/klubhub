import { updateTemplate } from '../../_state'

export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid template id' })
  }
  const { updatedAt, ...patch } = (await readBody(event)) ?? {}
  if (typeof updatedAt !== 'string' || updatedAt === '') {
    throw createError({ statusCode: 422, statusMessage: 'updatedAt is required' })
  }
  const result = updateTemplate(id, patch, updatedAt)
  if (!result.ok) {
    if (result.reason === 'conflict') {
      throw createError({ statusCode: 409, statusMessage: 'template changed since it was read' })
    }
    throw createError({ statusCode: 404, statusMessage: 'template not found' })
  }
  return { data: result.value }
})
