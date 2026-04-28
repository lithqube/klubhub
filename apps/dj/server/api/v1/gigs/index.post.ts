export default defineEventHandler(async (event) => {
  const body = await readBody(event)

  try {
    const data = await $fetch('/api/v1/gigs', {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'POST',
      body,
      headers: {
        'Content-Type': 'application/json',
      },
    })
    setResponseStatus(event, 201)
    return data
  } catch (err: any) {
    throw createError({
      statusCode: err.response?.status || 500,
      statusMessage: err.message || 'Failed to create gig',
    })
  }
})