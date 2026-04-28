export default defineEventHandler(async (event) => {
  const query = getQuery(event)

  try {
    const data = await $fetch('/api/v1/venues', {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'GET',
      params: query.q ? { q: String(query.q) } : undefined,
    })
    return data
  } catch (err: any) {
    throw createError({
      statusCode: err.response?.status || 500,
      statusMessage: err.message || 'Failed to fetch venues',
    })
  }
})