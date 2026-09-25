import { badRequest, invoices, serialize } from '../-mockDb'

const STATUSES = ['draft', 'issued', 'paid', 'cancelled', 'credited', 'corrected']
const KINDS = ['invoice', 'credit_note']
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export default defineEventHandler((event) => {
  const q = getQuery(event)
  const errs: string[] = []
  if (q.status && !STATUSES.includes(String(q.status))) errs.push('status: unknown status')
  if (q.kind && !KINDS.includes(String(q.kind))) errs.push('kind: unknown kind')
  if (q.gig_id && !UUID.test(String(q.gig_id))) errs.push('gig_id: must be a UUID')
  if (errs.length) return badRequest(event, errs)
  const list = [...invoices.values()]
    .filter((i) => !q.status || i.status === q.status)
    .filter((i) => !q.gig_id || i.gig_id === q.gig_id)
    .filter((i) => !q.kind || i.kind === q.kind)
    .filter((i) => !q.currency || i.currency === String(q.currency).toUpperCase())
    .sort((a, b) => b.created_at.localeCompare(a.created_at))
    .slice(0, 100)
    .map(serialize)
  return { data: list }
})
