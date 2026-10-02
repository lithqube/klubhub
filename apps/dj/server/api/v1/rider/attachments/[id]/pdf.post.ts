import { getAttachment } from '../../_state'

// Tiny minimal-PDF placeholder so the frontend download button works in
// dev. Real PDFs come from the Go API in production. The bytes below are
// a 1-page "PDF-1.4" stub that opens in any viewer.
const PLACEHOLDER_PDF = Buffer.from(
  '%PDF-1.4\n' +
  '1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n' +
  '2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n' +
  '3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n' +
  '4 0 obj<</Length 90>>stream\nBT /F1 18 Tf 60 780 Td (RIDER (mock dev PDF)) Tj\n0 -30 Td /F1 12 Tf (Set NUXT_PUBLIC_API_BASE to enable real export.) Tj ET\nendstream\nendobj\n' +
  '5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n' +
  'xref\n0 6\n0000000000 65535 f \n0000000009 00000 n \n0000000054 00000 n \n0000000099 00000 n \n0000000214 00000 n \n0000000354 00000 n \n' +
  'trailer<</Size 6/Root 1 0 R>>\nstartxref\n420\n%%EOF',
)

export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  if (!id) {
    throw createError({ statusCode: 400, statusMessage: 'invalid attachment id' })
  }
  const a = getAttachment(id)
  if (!a) {
    throw createError({ statusCode: 404, statusMessage: 'attachment not found' })
  }
  setResponseStatus(event, 201)
  // Bare envelope shape matches epk /export (no {data:...}).
  return {
    id: a.id,
    downloadUrl: 'data:application/pdf;base64,' + PLACEHOLDER_PDF.toString('base64'),
    createdAt: new Date().toISOString(),
  }
})