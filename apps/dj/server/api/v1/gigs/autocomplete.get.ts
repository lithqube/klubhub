export default defineEventHandler(async (event) => {
  const query = getQuery(event)
  const q = String(query.q || '')

  if (q.length < 2) {
    return []
  }

  try {
    const data = await $fetch(`/api/v1/gigs/autocomplete?q=${encodeURIComponent(q)}&limit=5`, {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'GET',
    })
    return data
  } catch (err: any) {
    throw createError({
      statusCode: err.response?.status || 500,
      statusMessage: err.message || 'Failed to fetch gig autocomplete',
    })
  }
})