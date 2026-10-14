// /api/v1/rider/* — served by the shared rider mock (shared/rider-mock), the
// same rules as the Nitro dev mocks and the Go API: update tokens, limits,
// unique template names, one rider per live gig.

import type { Failure, Result } from '../../../shared/rider-mock/db'
import type { DemoRouter } from '../router'
import { json, noContent } from '../router'
import { pdfBlob, type PdfLine } from '../lib/pdf'
import type { DemoContext, DemoRequest, DemoResponse } from '../types'

const CODES: Record<Failure['status'], string> = {
  400: 'bad_request',
  404: 'not_found',
  409: 'conflict',
  422: 'validation_failed',
}

/** The Go rider API's error shape: {error: <code>, message: <text>}. */
function failure(f: Failure): DemoResponse {
  return { status: f.status, body: { error: CODES[f.status], message: f.message } }
}

const object = (body: unknown): Record<string, unknown> =>
  body && typeof body === 'object' && !(body instanceof FormData) ? (body as Record<string, unknown>) : {}

function reply<T>(r: Result<T>, status: number, wrap = true): DemoResponse {
  if (!r.ok) return failure(r)
  return status === 204 ? noContent() : { status, body: wrap ? { data: r.value } : r.value }
}

function exportPdf(c: DemoContext, attachmentId: string): Blob | null {
  const att = c.rider.getAttachment(attachmentId)
  if (!att) return null
  const gig = c.state.gigs.find((g) => g.id === att.gigId)
  const where = [gig?.venue, [gig?.city, gig?.country].filter(Boolean).join(', ')].filter(Boolean).join(' - ')
  const lines: PdfLine[] = [
    { text: String(c.state.settings.dj_name || 'Sam Example').toUpperCase(), size: 22, bold: true },
    { text: where || 'Venue TBA', size: 12 },
    { text: gig?.date ? gig.date.slice(0, 10) : '', size: 10 },
    { text: 'Rider - KlubHub DJ demo export (fictional content)', size: 9 },
    { text: ' ' },
  ]
  const section = (title: string, body: string) => {
    if (body.trim() === '') return
    lines.push({ text: title, size: 13, bold: true }, { text: body }, { text: ' ' })
  }
  section('TECHNICAL', att.technical)
  section('HOSPITALITY', att.hospitality)
  section('BACKLINE', att.backline)
  section('OTHER NOTES', att.otherNotes)
  return pdfBlob(lines)
}

export function registerRider(r: DemoRouter): void {
  // ── templates ──
  r.on('GET', '/api/v1/rider/templates', (_req, c) => json({ data: c.rider.listTemplates() }))
  r.on('POST', '/api/v1/rider/templates', (req, c) => reply(c.rider.createTemplate(object(req.body)), 201))
  r.on('GET', '/api/v1/rider/templates/:id', (req, c) => {
    const t = c.rider.getTemplate(req.params.id!)
    return t ? json({ data: t }) : failure({ ok: false, status: 404, message: 'rider record not found' })
  })
  r.on('PUT', '/api/v1/rider/templates/:id', (req: DemoRequest & { params: Record<string, string> }, c) => {
    const { updatedAt, ...patch } = object(req.body)
    return reply(c.rider.updateTemplate(req.params.id!, patch, updatedAt), 200)
  })
  r.on('DELETE', '/api/v1/rider/templates/:id', (req, c) => reply(c.rider.deleteTemplate(req.params.id!), 204))

  // ── attachments ──
  // Missing is 200 {data: null}, not 404, like the real API.
  r.on('GET', '/api/v1/rider/attachments/by-gig/:gigId', (req, c) => json({ data: c.rider.getAttachmentByGig(req.params.gigId!) }))
  r.on('POST', '/api/v1/rider/attachments', (req, c) => {
    const body = object(req.body)
    const gigExists = typeof body.gigId === 'string' && c.state.gigs.some((g) => g.id === body.gigId)
    return reply(c.rider.createAttachment(body, gigExists), 201)
  })
  r.on('GET', '/api/v1/rider/attachments/:id', (req, c) => {
    const a = c.rider.getAttachment(req.params.id!)
    return a ? json({ data: a }) : failure({ ok: false, status: 404, message: 'rider record not found' })
  })
  r.on('PUT', '/api/v1/rider/attachments/:id', (req, c) => {
    const { updatedAt, ...patch } = object(req.body)
    return reply(c.rider.updateAttachment(req.params.id!, patch, updatedAt), 200)
  })
  r.on('DELETE', '/api/v1/rider/attachments/:id', (req, c) => reply(c.rider.deleteAttachment(req.params.id!), 204))
  r.on('POST', '/api/v1/rider/attachments/:id/pdf', (req, c) => {
    const blob = exportPdf(c, req.params.id!)
    if (!blob) return failure({ ok: false, status: 404, message: 'rider record not found' })
    // Bare envelope (no {data}), like the Go API and the EPK export.
    return json({ id: req.params.id, downloadUrl: c.objectUrl(blob), createdAt: c.now().toISOString() }, 201)
  })
}
