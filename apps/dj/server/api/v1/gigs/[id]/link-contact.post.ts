export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  const body = await readBody(event)

  try {
    const data = await $fetch(`/api/v1/gigs/${id}/link-contact`, {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'POST',
      body,
      headers: {
        'Content-Type': 'application/json',
      },
    })
    return data
  } catch (err: any) {
    throw createError({
      statusCode: err.response?.status || 500,
      statusMessage: err.message || 'Failed to link contact to gig',
    })
  }
})