// /api/v1/finance/* — served by the shared finance mock (shared/finance-mock),
// the same rules as the Nitro dev mocks.

import type { MockGig } from '../../../shared/finance-mock/db'
import * as att from '../../../shared/finance-mock/attachments'
import * as ops from '../../../shared/finance-mock/ops'
import { pdfBlob } from '../lib/pdf'
import { fail, party, type MockResult } from '../../../shared/finance-mock/rules'
import { gigLabel } from '../../../shared/finance-mock/seed'
import { parseMoney } from '../../utils/money'
import type { DemoRouter } from '../router'
import type { DemoRequest, DemoResponse, DemoState } from '../types'

/** Finance sees demo gigs through this lookup (fee, date, customer). */
export function financeLookup(state: DemoState) {
  return (id: string): MockGig | undefined => {
    const g = state.gigs.find((x) => x.id === id)
    if (!g) return undefined
    const currency = g.fee_currency || 'EUR'
    const customer = state.customers[id] ?? party({
      legal_name: g.promoter_name, email: g.promoter_email, country: (g.country || '').toUpperCase(),
    })
    return {
      id,
      date: g.date.slice(0, 10),
      label: gigLabel(g.venue, g.event_name),
      fee_minor: parseMoney(String(g.fee_amount ?? 0), currency) ?? 0,
      currency,
      customer: structuredClone(customer),
    }
  }
}

const res = (r: MockResult): DemoResponse => ({ status: r.status, body: r.body })
const body = (req: DemoRequest) => (req.body && typeof req.body === 'object' ? req.body : {}) as Record<string, unknown>
const query = (req: DemoRequest) => Object.fromEntries(req.query)

export function registerFinance(r: DemoRouter): void {
  const B = '/api/v1/finance'
  r.on('GET', `${B}/invoices`, (req, c) => res(ops.listInvoices(c.finance, query(req))))
    .on('POST', `${B}/invoices`, (req, c) => res(ops.createInvoice(c.finance, req.body as Record<string, unknown> | null)))
    .on('GET', `${B}/invoices/summaries`, (_req, c) => res(ops.summaries(c.finance)))
    .on('GET', `${B}/invoices/tax-suggestion`, (req, c) => res(ops.taxSuggestion(c.finance, query(req))))
    .on('GET', `${B}/invoices/tax-notes`, (_req, c) => res(ops.taxNotes(c.finance)))
    .on('GET', `${B}/invoices/:id`, (req, c) => res(ops.getInvoice(c.finance, req.params.id!)))
    .on('PUT', `${B}/invoices/:id`, (req, c) => res(ops.updateInvoice(c.finance, req.params.id!, body(req))))
    .on('GET', `${B}/invoices/:id/pdf`, (req, c) => {
      const pdf = ops.invoicePdfLines(c.finance, req.params.id!)
      if (!pdf) return res(fail(404, 'not_found', 'invoice not found'))
      return {
        status: 200,
        blob: pdfBlob(pdf.lines),
        headers: { 'content-disposition': `attachment; filename="${pdf.name}"` },
      }
    })
    .on('GET', `${B}/invoices/:id/einvoice-check`, (req, c) => res(ops.einvoiceCheck(c.finance, req.params.id!, query(req).format ?? '')))
    .on('GET', `${B}/invoices/:id/einvoice`, (req, c) => res(ops.einvoiceCheck(c.finance, req.params.id!, query(req).format ?? '')))
    .on('POST', `${B}/invoices/:id/email`, (req, c) => res(ops.emailInvoice(c.finance, req.params.id!)))
    .on('GET', `${B}/documents`, (req, c) => res(ops.listDocuments(c.finance, query(req).owner_id ?? '')))
    .on('GET', `${B}/invoices/:id/issue-check`, (req, c) => res(ops.issueCheck(c.finance, req.params.id!)))
    .on('POST', `${B}/invoices/:id/issue`, (req, c) => res(ops.issueInvoice(c.finance, req.params.id!, body(req))))
    .on('POST', `${B}/invoices/:id/cancel`, (req, c) => res(ops.cancelInvoice(c.finance, req.params.id!, body(req))))
    .on('POST', `${B}/invoices/:id/pay`, (req, c) => res(ops.payInvoice(c.finance, req.params.id!, body(req))))
    .on('POST', `${B}/invoices/:id/credit-note`, (req, c) => res(ops.creditNote(c.finance, req.params.id!, body(req))))
    .on('POST', `${B}/invoices/:id/correct`, (req, c) => res(ops.correctInvoice(c.finance, req.params.id!, body(req))))
    .on('GET', `${B}/invoices/:id/payments`, (req, c) => res(ops.listPayments(c.finance, req.params.id!)))
    .on('POST', `${B}/invoices/:id/payments`, (req, c) => res(ops.createPayment(c.finance, req.params.id!, body(req))))
    .on('GET', `${B}/payments/:id`, (req, c) => res(ops.getPayment(c.finance, req.params.id!)))
    .on('PUT', `${B}/payments/:id`, (req, c) => res(ops.updatePayment(c.finance, req.params.id!, body(req))))
    .on('GET', `${B}/billing-profile`, (_req, c) => res(ops.getBillingProfile(c.finance)))
    .on('PUT', `${B}/billing-profile`, (req, c) => res(ops.updateBillingProfile(c.finance, body(req))))
    // Phase 5: earnings ledger + reconciliations.
    .on('GET', `${B}/entries`, (req, c) => res(ops.listEntries(c.finance, query(req))))
    .on('POST', `${B}/entries`, (req, c) => res(ops.createEntry(c.finance, req.body as Record<string, unknown> | null)))
    .on('GET', `${B}/entries/:id`, (req, c) => res(ops.getEntry(c.finance, req.params.id!)))
    .on('PUT', `${B}/entries/:id`, (req, c) => res(ops.updateEntry(c.finance, req.params.id!, body(req))))
    .on('DELETE', `${B}/entries/:id`, (req, c) => res(ops.deleteEntry(c.finance, req.params.id!, body(req))))
    .on('POST', `${B}/entries/:id/void`, (req, c) => res(ops.voidEntry(c.finance, req.params.id!, body(req))))
    // Receipts on entries (multipart upload; the file part is read into memory).
    .on('GET', `${B}/entries/:id/attachments`, (req, c) => res(att.listAttachments(c.finance, req.params.id!)))
    .on('POST', `${B}/entries/:id/attachments`, async (req, c) => {
      const part = req.body instanceof FormData ? req.body.get('file') : null
      if (!(req.body instanceof FormData)) {
        return res(fail(400, 'bad_request', 'send the file as multipart/form-data in a "file" part'))
      }
      const file = part instanceof File ? { name: part.name, bytes: new Uint8Array(await part.arrayBuffer()) } : null
      return res(att.addAttachment(c.finance, req.params.id!, file))
    })
    .on('GET', `${B}/entries/:id/attachments/:aid`, (req, c) => {
      const file = att.readAttachment(c.finance, req.params.id!, req.params.aid!, req.query.get('inline') === '1')
      if (!att.isAttachmentFile(file)) return res(file)
      return {
        status: 200,
        blob: new Blob([file.bytes as BlobPart], { type: file.mime }),
        headers: { 'content-disposition': `${file.disposition}; filename="${file.filename.replace(/[^\x20-\x7e]|["\\]/g, '_')}"` },
      }
    })
    .on('DELETE', `${B}/entries/:id/attachments/:aid`, (req, c) => res(att.removeAttachment(c.finance, req.params.id!, req.params.aid!)))
    .on('GET', `${B}/summary`, (req, c) => res(ops.summaryEntries(c.finance, query(req))))
    .on('GET', `${B}/profit-loss`, (req, c) => res(ops.profitLossScope(c.finance, query(req))))
    .on('GET', `${B}/reconciliations`, (req, c) => res(ops.getReconciliationByGig(c.finance, query(req))))
    .on('POST', `${B}/reconciliations/:id/resolve`, (req, c) => res(ops.resolveReconciliation(c.finance, req.params.id!, body(req))))
}
