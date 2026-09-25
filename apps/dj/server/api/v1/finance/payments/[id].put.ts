import type { PaymentRecordStatus } from '../../../../../app/types/finance'
import { fail, invoices, payments, stamp } from '../-mockDb'

const STATUSES: PaymentRecordStatus[] = ['pending', 'completed', 'failed', 'refunded']

export default defineEventHandler(async (event) => {
  const p = payments.get(getRouterParam(event, 'id') ?? '')
  if (!p) return fail(event, 404, 'not_found', 'payment not found')
  const body = ((await readBody(event).catch(() => null)) ?? {}) as Record<string, unknown>
  if (typeof body.updated_at !== 'string' || !body.updated_at) return fail(event, 400, 'bad_request', 'updated_at is required')
  if (body.updated_at !== p.updated_at) return fail(event, 409, 'conflict', 'payment updated by another writer')
  const status = body.status as PaymentRecordStatus
  if (!STATUSES.includes(status)) {
    return fail(event, 400, 'validation_failed', 'validation failed: status: must be one of pending, completed, failed, refunded')
  }
  const inv = invoices.get(p.invoice_id)
  if (!inv || (inv.status !== 'issued' && inv.status !== 'paid')) {
    return fail(event, 422, 'invoice_not_payable', 'invoice does not accept payments in its current status')
  }
  p.status = status
  p.method = String(body.method ?? p.method)
  p.reference = String(body.reference ?? p.reference)
  p.received_at = typeof body.received_at === 'string' ? body.received_at : p.received_at
  p.updated_at = stamp()
  return { data: p }
})
