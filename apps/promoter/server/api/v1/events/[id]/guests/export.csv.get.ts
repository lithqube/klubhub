import { findEvent, guestLists, allocations, guestPage } from '../../../-mockDb'
import { toCsv } from '~/utils/guests'
export default defineEventHandler((event) => {
  const e = findEvent(getRouterParam(event, 'id')!)
  const q = getQuery(event)
  const page = guestPage(e.id, { status: q.status as string | undefined, list_id: q.list_id as string | undefined }, false)
  const rows = page.guests.map(g => [g.name, g.plus_n, g.status, guestLists.find(l => l.id === g.list_id)?.name ?? '',
    allocations.find(a => a.id === g.allocation_id)?.label ?? '', g.email, g.phone, g.note])
  setHeader(event, 'Content-Type', 'text/csv; charset=utf-8')
  setHeader(event, 'Content-Disposition', `attachment; filename="${e.slug}-guests.csv"`)
  return '\ufeff' + toCsv([['name', 'plus_n', 'status', 'list', 'allocation', 'email', 'phone', 'note'], ...rows])
})
