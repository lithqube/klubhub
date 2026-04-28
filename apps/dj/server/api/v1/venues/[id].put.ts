export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  const body = await readBody(event)

  try {
    const data = await $fetch(`/api/v1/venues/${id}`, {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'PUT',
      body,
      headers: {
        'Content-Type': 'application/json',
      },
    })
    return data
  } catch (err: any) {
    const status = err.response?.status || 500
    if (status === 409) {
      throw createError({
        statusCode: 409,
        statusMessage: 'Conflict: venue was modified by another request',
      })
    }
    throw createError({
      statusCode: status,
      statusMessage: err.message || 'Failed to update venue',
    })
  }
})