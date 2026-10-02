import { updateAttachment } from '../../_state'

export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid attachment id' })
  }
  const { updatedAt, ...patch } = (await readBody(event)) ?? {}
  if (typeof updatedAt !== 'string' || updatedAt === '') {
    throw createError({ statusCode: 422, statusMessage: 'updatedAt is required' })
  }
  const result = updateAttachment(id, patch, updatedAt)
  if (!result.ok) {
    if (result.reason === 'conflict') {
      throw createError({ statusCode: 409, statusMessage: 'attachment changed since it was read' })
    }
    throw createError({ statusCode: 404, statusMessage: 'attachment not found' })
  }
  return { data: result.value }
})
