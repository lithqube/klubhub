export default defineEventHandler(async (event) => {
  try {
    return await $fetch('/api/v1/releases', {
      baseURL: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080',
      method: 'GET',
    })
  } catch (err: any) {
    throw createError({
      statusCode: err.response?.status || 500,
      statusMessage: err.message || 'Failed to fetch releases',
    })
  }
})
