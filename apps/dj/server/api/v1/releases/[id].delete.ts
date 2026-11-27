export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  try {
    await $fetch(`/api/v1/releases/${id}`, {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'DELETE',
    })
    return { ok: true }
  } catch (err: any) {
    throw createError({
      statusCode: err.response?.status || 500,
      statusMessage: err.message || 'Failed to delete the release',
    })
  }
})
