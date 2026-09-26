import { listView, findList } from '../../../../../-mockDb'
export default defineEventHandler((event) => {
  const eventId = getRouterParam(event, 'id')!
  const l = findList(eventId, getRouterParam(event, 'listId')!)
  return listView(eventId, l.id).allocations
})
