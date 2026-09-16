// Plan B.9 / D — dev-only mock handler. The directory
// apps/dj/server/api/v1/ is renamed to apps/dj/server/api/v1.mock/ for
// production NODE_ENV (see apps/dj/nuxt.config.ts: hooks.build:before),
// so this file is not compiled into the published bundle. Dev and
// staging builds keep it and serve fake-200 echoes for the frontend's
// offline-mode workflow.

export default defineEventHandler(async (event) => {
  const body = await readBody(event)
  // In mock mode, echo back the patch merged with base EPK data
  return {
    id: 'epk-001',
    bioShort: body.bioShort ?? '',
    bioLong: body.bioLong ?? '',
    techRider: body.techRider ?? '',
    stagePlotPath: body.stagePlotPath ?? '',
    gigHighlights: body.gigHighlights ?? [],
    pressQuotes: body.pressQuotes ?? [],
    photoPaths: body.photoPaths ?? [],
    sectionVisibility: body.sectionVisibility ?? {},
    createdAt: '2025-06-01T10:00:00Z',
    updatedAt: new Date().toISOString(),
  }
})
