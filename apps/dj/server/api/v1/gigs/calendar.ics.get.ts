// Plan B.5 / D — server-only secret. The client-side bundle has NO
// access to this secret (see apps/dj/nuxt.config.ts: runtimeConfig.public
// has no icalSecret entry). In production builds, this file is part
// of the apps/dj/server/api/v1/ directory which is renamed to
// v1.mock/ and excluded from the production bundle (see hooks.build:before
// in nuxt.config.ts). In dev/staging builds the handler proxies the
// calendar feed to the Go API using the server-side-only secret.

export default defineEventHandler(async (event) => {
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
      statusMessage: 'Calendar feed requires NUXT_PUBLIC_API_BASE',
    })
  }

  try {
    const data = await $fetch(`/api/v1/gigs/calendar.ics?secret=${encodeURIComponent(icalSecret)}`, {
      baseURL: apiBase,
      method: 'GET',
      responseType: 'blob',
    })

    setHeader(event, 'Content-Type', 'text/calendar; charset=utf-8')
    setHeader(event, 'Content-Disposition', 'attachment; filename=klubhub-gigs.ics')

    return data
  } catch (err: any) {
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
