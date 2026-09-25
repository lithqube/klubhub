import type { PaymentKind } from '../../../../../../app/types/finance'
import { addPayment, balances, fail, findInvoice } from '../../-mockDb'

const KINDS: PaymentKind[] = ['deposit', 'payment', 'refund']

export default defineEventHandler(async (event) => {
  const inv = findInvoice(event)
  if (!inv) return fail(event, 404, 'not_found', 'invoice not found')
  const body = ((await readBody(event).catch(() => null)) ?? {}) as Record<string, unknown>
  const amount = Number(body.amount_minor)
  const kind = body.kind as PaymentKind
  const errs: string[] = []
  if (!Number.isInteger(amount) || amount <= 0) errs.push('amount_minor: must be positive')
  if (!KINDS.includes(kind)) errs.push('kind: must be one of deposit, payment, refund')
  if (errs.length) return fail(event, 400, 'validation_failed', 'validation failed: ' + errs.join('; '))
  if (inv.kind !== 'invoice' || (inv.status !== 'issued' && inv.status !== 'paid')) {
    return fail(event, 422, 'invoice_not_payable', 'invoice does not accept payments in its current status')
  }
  if (body.currency !== inv.currency) {
    return fail(event, 400, 'validation_failed', `validation failed: currency: must match invoice currency ${inv.currency}`)
  }
  const b = balances(inv)
  const limit = kind === 'refund' ? b.received_minor - b.pending_refunds : b.outstanding_minor
  if (amount > limit) return fail(event, 422, 'exceeds_balance', 'payment exceeds invoice balance')
  const p = addPayment(inv.id, {
    currency: inv.currency,
    amount_minor: amount,
    kind,
    method: String(body.method ?? ''),
    reference: String(body.reference ?? ''),
    received_at: typeof body.received_at === 'string' ? body.received_at : null,
  })
  setResponseStatus(event, 201)
  return { data: p }
})
