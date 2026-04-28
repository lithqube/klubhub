export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  const icalSecret = process.env.ICAL_SECRET || process.env.NUXT_PUBLIC_ICAL_SECRET || ''

  if (!id) {
    throw createError({
      statusCode: 400,
      statusMessage: 'Gig ID is required',
    })
  }

  try {
    const data = await $fetch(`/api/v1/gigs/${id}/pdf?secret=${encodeURIComponent(icalSecret)}`, {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'GET',
      responseType: 'blob',
    })

    // Set headers for file download
    setHeader(event, 'Content-Type', 'application/pdf')
    setHeader(event, 'Content-Disposition', 'attachment; filename=booking-confirmation.pdf')

    return data
  } catch (err: any) {
    // Forward error status codes
    if (err.response?.status) {
      throw createError({
        statusCode: err.response.status,
        statusMessage: err.message || 'Failed to fetch booking confirmation PDF',
      })
    }
    throw createError({
      statusCode: 500,
      statusMessage: err.message || 'Failed to fetch booking confirmation PDF',
    })
  }
})
