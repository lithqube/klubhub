import * as ops from '../../../../../shared/finance-mock/ops'
import { db, learnGig, send } from '../-mockDb'

export default defineEventHandler(async (event) => {
  const body = (await readBody(event).catch(() => null)) as Record<string, unknown> | null
  if (body && typeof body.gig_id === 'string' && body.gig_id) await learnGig(body.gig_id)
  return send(event, ops.createInvoice(db, body))
})
