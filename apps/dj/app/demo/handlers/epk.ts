// /api/v1/epk/* — press kit content, photos (data URLs, size-capped) and
// PDF export (a generated text PDF, created in the browser).

import { uuid } from '../../../shared/finance-mock/rules'
import type { EPKContent, UpdateEPKContentRequest } from '../../types/epk'
import { pdfBlob, type PdfLine } from '../lib/pdf'
import { assetBytes, dataUrlToBlob, imageDataUrl, MAX_ASSET_TOTAL, TooLargeError } from '../lib/util'
import type { DemoRouter } from '../router'
import { apiError, json, noContent, notFound } from '../router'
import type { DemoContext } from '../types'

const IMAGE_TYPES = ['image/jpeg', 'image/png', 'image/webp']
const CONTENT_KEYS: (keyof UpdateEPKContentRequest)[] = [
  'bioShort', 'bioLong', 'techRider', 'stagePlotPath', 'gigHighlights', 'pressQuotes', 'photoPaths', 'sectionVisibility',
]

function view(c: DemoContext): EPKContent {
  const e = c.state.epk
  return { ...e, photoUrls: Object.fromEntries(e.photoPaths.map((p) => [p, c.state.assets[p] ?? ''])) }
}

async function storeImage(c: DemoContext, file: unknown, folder: string): Promise<{ path: string; url: string }> {
  if (!(file instanceof Blob)) throw new TypeError('missing file')
  if (!IMAGE_TYPES.includes(file.type)) throw new TypeError('Use a JPEG, PNG or WebP image.')
  const url = await imageDataUrl(file)
  if (assetBytes(c.state.assets) + url.length > MAX_ASSET_TOTAL) {
    throw new TooLargeError('Demo image storage is full (about 3 MB in this browser). Remove a photo or reset the demo.')
  }
  const path = `demo/epk/${folder}/${uuid()}`
  c.state.assets[path] = url
  return { path, url }
}

const uploadError = (e: unknown) => apiError(e instanceof TooLargeError ? 413 : 400, e instanceof Error ? e.message : 'upload failed')

function exportPdf(c: DemoContext): Blob {
  const e = c.state.epk
  const s = c.state.settings
  const shown = (k: string) => e.sectionVisibility[k] !== false
  const lines: PdfLine[] = [
    { text: String(s.dj_name || 'Sam Example'), size: 22, bold: true },
    { text: 'Electronic press kit - KlubHub DJ demo export (fictional content)', size: 9 },
    { text: ' ' },
  ]
  const section = (title: string, body: string[]) => {
    if (body.length === 0) return
    lines.push({ text: title.toUpperCase(), size: 13, bold: true }, ...body.map((text) => ({ text })), { text: ' ' })
  }
  if (shown('bio')) section('Bio', [e.bioLong || e.bioShort].filter(Boolean))
  if (shown('gig_highlights')) section('Gig highlights', e.gigHighlights.map((h) => `- ${h}`))
  section('Press', e.pressQuotes.map((q) => `"${q.text}" - ${q.source}`))
  if (shown('tech_rider')) section('Tech rider', e.techRider ? [e.techRider] : [])
  const links = Object.entries((s.social_links ?? {}) as Record<string, string>).filter(([, v]) => v)
  if (shown('social_links')) section('Links', links.map(([k, v]) => `${k}: ${v}`))
  section('Booking', [String(s.contact_info || '')].filter(Boolean))
  if (shown('photos') && e.photoPaths.length) section('Photos', [`${e.photoPaths.length} press photos (included in the full app's PDF).`])
  return pdfBlob(lines)
}

export function registerEpk(r: DemoRouter): void {
  r.on('GET', '/api/v1/epk/content', (_req, c) => json({ data: view(c) }))

  r.on('PUT', '/api/v1/epk/content', (req, c) => {
    const patch = (req.body ?? {}) as Record<string, unknown>
    const e = c.state.epk as unknown as Record<string, unknown>
    for (const k of CONTENT_KEYS) if (patch[k] !== undefined) e[k] = structuredClone(patch[k])
    c.state.epk.updatedAt = c.now().toISOString()
    return json({ data: view(c) })
  })

  r.on('POST', '/api/v1/epk/photos', async (req, c) => {
    if (c.state.epk.photoPaths.length >= 20) return apiError(400, 'photo limit reached (20)')
    try {
      const { path, url } = await storeImage(c, req.body instanceof FormData ? req.body.get('photo') : null, 'photos')
      c.state.epk.photoPaths.push(path)
      return json({ path, url }, 201)
    } catch (e) {
      return uploadError(e)
    }
  })

  r.on('DELETE', '/api/v1/epk/photos/:path', (req, c) => {
    const path = req.params.path!
    if (!c.state.epk.photoPaths.includes(path)) return notFound('photo not found')
    c.state.epk.photoPaths = c.state.epk.photoPaths.filter((p) => p !== path)
    Reflect.deleteProperty(c.state.assets, path)
    return noContent()
  })

  r.on('POST', '/api/v1/epk/stage-plot', async (req, c) => {
    try {
      const { path } = await storeImage(c, req.body instanceof FormData ? req.body.get('stagePlot') : null, 'stage-plot')
      if (c.state.epk.stagePlotPath) Reflect.deleteProperty(c.state.assets, c.state.epk.stagePlotPath)
      c.state.epk.stagePlotPath = path
      return json({ path }, 201)
    } catch (e) {
      return uploadError(e)
    }
  })

  r.on('POST', '/api/v1/epk/export', (_req, c) => {
    const createdAt = c.now().toISOString()
    const id = uuid()
    c.state.exports.unshift({ id, minioPath: `demo/epk/exports/${id}.pdf`, createdAt })
    c.state.exports = c.state.exports.slice(0, 10)
    return json({ id, downloadUrl: c.objectUrl(exportPdf(c)), createdAt }, 201)
  })

  // Export files are regenerated from the current content on each listing.
  r.on('GET', '/api/v1/epk/exports', (_req, c) => {
    const url = c.state.exports.length ? c.objectUrl(exportPdf(c)) : ''
    return json({ data: c.state.exports.map((x) => ({ ...x, downloadUrl: url })) })
  })

  r.on('DELETE', '/api/v1/epk/exports/:id', (req, c) => {
    const before = c.state.exports.length
    c.state.exports = c.state.exports.filter((x) => x.id !== req.params.id)
    return before === c.state.exports.length ? notFound('export not found') : noContent()
  })

  r.on('GET', '/api/v1/storage/proxy', (req, c) => {
    const url = c.state.assets[req.query.get('path') ?? '']
    return url ? { status: 200, blob: dataUrlToBlob(url) } : notFound('object not found')
  })
}
