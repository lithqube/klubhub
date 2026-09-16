// Plan B.5 / D — server-only secret. See calendar.ics.get.ts for the
// full rationale. This file lives under apps/dj/server/api/v1/ which is
// tree-shaken from production builds (renamed to v1.mock/).

export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')

  if (!id) {
    throw createError({
      statusCode: 400,
      statusMessage: 'Gig ID is required',
    })
  }

  const icalSecret = process.env.ICAL_SECRET || ''
  const apiBase = process.env.NUXT_PUBLIC_API_BASE || ''

  if (!icalSecret) {
    throw createError({
      statusCode: 503,
      statusMessage: 'ICAL_SECRET not configured on the server',
    })
  }

  if (!apiBase) {
    throw createError({
      statusCode: 503,
      statusMessage: 'Booking PDF requires NUXT_PUBLIC_API_BASE',
    })
  }

  try {
    const data = await $fetch(`/api/v1/gigs/${id}/pdf?secret=${encodeURIComponent(icalSecret)}`, {
      baseURL: apiBase,
      method: 'GET',
      responseType: 'blob',
    })

    setHeader(event, 'Content-Type', 'application/pdf')
    setHeader(event, 'Content-Disposition', 'attachment; filename=booking-confirmation.pdf')

    return data
  } catch (err: any) {
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
