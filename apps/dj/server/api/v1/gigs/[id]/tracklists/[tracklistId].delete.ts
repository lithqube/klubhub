export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  const tracklistId = getRouterParam(event, 'tracklistId')

  if (!id || !tracklistId) {
    throw createError({ statusCode: 400, statusMessage: 'Missing gig or tracklist id' })
  }

  try {
    await $fetch(`/api/v1/gigs/${id}/tracklists/${tracklistId}`, {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'DELETE',
    })
  } catch (err: any) {
    throw createError({
      statusCode: err.response?.status || 500,
      statusMessage: err.message || 'Failed to unlink tracklist from gig',
    })
  }
})
