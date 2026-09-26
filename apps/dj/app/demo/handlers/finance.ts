// /api/v1/finance/* — served by the shared finance mock (shared/finance-mock),
// the same rules as the Nitro dev mocks.

import type { MockGig } from '../../../shared/finance-mock/db'
import * as ops from '../../../shared/finance-mock/ops'
import { party, type MockResult } from '../../../shared/finance-mock/rules'
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
    .on('GET', `${B}/invoices/:id`, (req, c) => res(ops.getInvoice(c.finance, req.params.id!)))
    .on('PUT', `${B}/invoices/:id`, (req, c) => res(ops.updateInvoice(c.finance, req.params.id!, body(req))))
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
}
