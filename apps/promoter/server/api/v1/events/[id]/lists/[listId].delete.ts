import { findList, guestLists, guests } from '../../../-mockDb'
export default defineEventHandler((event) => {
  const l = findList(getRouterParam(event, 'id')!, getRouterParam(event, 'listId')!)
  const n = guests.filter(g => g.list_id === l.id).length
  if (n > 0 && getQuery(event).force !== 'true') throw createError({ statusCode: 409, data: { error: 'list_not_empty', guests: n } })
  for (let i = guests.length - 1; i >= 0; i--) if (guests[i]!.list_id === l.id) guests.splice(i, 1)
  guestLists.splice(guestLists.indexOf(l), 1)
  setResponseStatus(event, 204)
  return null
})
