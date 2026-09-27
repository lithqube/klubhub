import { findEvent, listBackAllocation, listBackRows } from '../../../-mockDb'
import { toCsv } from '~/utils/guests'
import { listBackFilename } from '~/utils/report'
export default defineEventHandler((event) => {
  const e = findEvent(getRouterParam(event, 'id')!)
  const id = String(getQuery(event).allocation_id ?? '')
  const rows = listBackRows(e.id, id)
  setHeader(event, 'Content-Type', 'text/csv; charset=utf-8')
  setHeader(event, 'Content-Disposition', `attachment; filename="${listBackFilename(e.slug, listBackAllocation(e.id, id).label)}"`)
  return '\ufeff' + toCsv(rows)
})
