export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')

  try {
    const data = await $fetch(`/api/v1/contacts/${id}`, {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'GET',
    })
    return data
  } catch (err: any) {
    throw createError({
      statusCode: err.response?.status || 500,
      statusMessage: err.message || 'Failed to fetch contact',
    })
  }
})