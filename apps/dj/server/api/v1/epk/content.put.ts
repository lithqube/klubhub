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
