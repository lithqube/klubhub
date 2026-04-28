export default defineEventHandler(async (event) => {
  const query = getQuery(event)
  const q = String(query.q || '')

  if (q.length < 2) {
    return []
  }

  const limit = parseInt(String(query.limit || '5'), 10)

  try {
    const data = await $fetch(`/api/v1/contacts/autocomplete?q=${encodeURIComponent(q)}&limit=${limit}`, {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'GET',
    })
    return data
  } catch (err: any) {
    throw createError({
      statusCode: err.response?.status || 500,
      statusMessage: err.message || 'Failed to fetch contact autocomplete',
    })
  }
})