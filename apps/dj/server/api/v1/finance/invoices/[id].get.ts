import { fail, findInvoice, lines, serialize } from '../-mockDb'

export default defineEventHandler((event) => {
  const inv = findInvoice(event)
  if (!inv) return fail(event, 404, 'not_found', 'invoice not found')
  return { data: serialize(inv), lines: lines.get(inv.id) ?? [] }
})
