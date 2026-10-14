// Frontend-only mock of PUT /api/v1/tracklists/:id (rename). Stateless like the
// other tracklist mocks: it validates like the Go API and echoes the result.
export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  const body = await readBody<{ title?: unknown }>(event)
  if (body?.title === undefined || body.title === null) {
    throw createError({ statusCode: 422, data: { error: 'title is required' } })
  }
  const title = String(body.title).trim()
  if (!title || [...title].length > 200 || title.includes('\0')) {
    throw createError({ statusCode: 422, data: { error: 'title must be 1-200 characters' } })
  }
  return { id, title, updatedAt: new Date().toISOString() }
})
