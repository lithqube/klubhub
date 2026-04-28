export default defineEventHandler(async (event) => {
  const icalSecret = process.env.ICAL_SECRET || process.env.NUXT_PUBLIC_ICAL_SECRET || ''

  try {
    const data = await $fetch(`/api/v1/gigs/calendar.ics?secret=${encodeURIComponent(icalSecret)}`, {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'GET',
      responseType: 'blob',
    })

    // Set headers for file download
    setHeader(event, 'Content-Type', 'text/calendar; charset=utf-8')
    setHeader(event, 'Content-Disposition', 'attachment; filename=klubhub-gigs.ics')

    return data
  } catch (err: any) {
    // If the backend returns an error (e.g., 403 for invalid secret), forward it
    if (err.response?.status) {
      throw createError({
        statusCode: err.response.status,
        statusMessage: err.message || 'Failed to fetch calendar',
      })
    }
    throw createError({
      statusCode: 500,
      statusMessage: err.message || 'Failed to fetch calendar',
    })
  }
})
