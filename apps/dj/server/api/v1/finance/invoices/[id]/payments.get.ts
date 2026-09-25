import { fail, findInvoice, payments } from '../../-mockDb'

export default defineEventHandler((event) => {
  const inv = findInvoice(event)
  if (!inv) return fail(event, 404, 'not_found', 'invoice not found')
  const list = [...payments.values()]
    .filter((p) => p.invoice_id === inv.id)
    .sort((a, b) => a.created_at.localeCompare(b.created_at))
  return { data: list }
})
