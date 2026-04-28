export default defineEventHandler(async (event) => {
  const query = getQuery(event)
  const params = new URLSearchParams()
  if (query.status) params.set('status', String(query.status))
  if (query.venue) params.set('venue', String(query.venue))
  if (query.city) params.set('city', String(query.city))
  if (query.fee_min) params.set('fee_min', String(query.fee_min))
  if (query.fee_max) params.set('fee_max', String(query.fee_max))
  if (query.from) params.set('from', String(query.from))
  if (query.to) params.set('to', String(query.to))

  const queryStr = params.toString()
  const url = `/api/v1/gigs${queryStr ? `?${queryStr}` : ''}`

  try {
    const data = await $fetch(url, {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'GET',
    })
    return data
  } catch (err: any) {
    throw createError({
      statusCode: err.response?.status || 500,
      statusMessage: err.message || 'Failed to fetch gigs',
    })
  }
})