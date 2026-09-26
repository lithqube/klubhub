import { guestPage } from '../../../-mockDb'
export default defineEventHandler((event) => {
  const q = getQuery(event)
  return guestPage(getRouterParam(event, 'id')!, { status: q.status as string | undefined, list_id: q.list_id as string | undefined })
})
