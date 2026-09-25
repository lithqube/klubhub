import { badRequest, checkToken, creditNoteFor, fail, findInvoice, serialize, stamp } from '../../-mockDb'

export default defineEventHandler(async (event) => {
  const inv = findInvoice(event)
  if (!inv) return fail(event, 404, 'not_found', 'invoice not found')
  const body = ((await readBody(event).catch(() => null)) ?? {}) as Record<string, unknown>
  const tokenErr = checkToken(event, inv, body.updated_at)
  if (tokenErr) return tokenErr
  if (inv.kind !== 'invoice' || (inv.status !== 'issued' && inv.status !== 'paid')) {
    return fail(event, 409, 'bad_state', 'only issued or paid invoices can be credited')
  }
  const reason = String(body.reason ?? '').trim()
  if (reason.length > 1000) return badRequest(event, ['reason: exceeds 1000 characters'])
  const cn = creditNoteFor(inv, reason)
  inv.status = 'credited'
  inv.updated_at = stamp()
  setResponseStatus(event, 201)
  return { data: { credit_note: serialize(cn), original: serialize(inv) } }
})
