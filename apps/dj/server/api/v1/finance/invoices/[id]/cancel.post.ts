import { checkToken, fail, findInvoice, serialize, stamp } from '../../-mockDb'

export default defineEventHandler(async (event) => {
  const inv = findInvoice(event)
  if (!inv) return fail(event, 404, 'not_found', 'invoice not found')
  const body = ((await readBody(event).catch(() => null)) ?? {}) as Record<string, unknown>
  const tokenErr = checkToken(event, inv, body.updated_at)
  if (tokenErr) return tokenErr
  if (inv.status !== 'draft') return fail(event, 409, 'bad_state', 'only drafts can be cancelled')
  inv.status = 'cancelled'
  inv.updated_at = stamp()
  return { data: serialize(inv) }
})
