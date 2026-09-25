import { checkToken, fail, findInvoice, serialize, stamp } from '../../-mockDb'

export default defineEventHandler(async (event) => {
  const inv = findInvoice(event)
  if (!inv) return fail(event, 404, 'not_found', 'invoice not found')
  const body = ((await readBody(event).catch(() => null)) ?? {}) as Record<string, unknown>
  const tokenErr = checkToken(event, inv, body.updated_at)
  if (tokenErr) return tokenErr
  if (inv.kind !== 'invoice' || inv.status !== 'issued') return fail(event, 409, 'bad_state', 'only issued invoices can be marked paid')
  inv.status = 'paid'
  inv.paid_at = typeof body.paid_at === 'string' && body.paid_at ? body.paid_at : stamp()
  inv.payment_ref = String(body.payment_ref ?? '')
  inv.updated_at = stamp()
  return { data: serialize(inv) }
})
