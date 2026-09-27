import type { ListInput } from '~/types/guest'
import { findList, guests, listView, validateListInput } from '../../../-mockDb'
export default defineEventHandler(async (event) => {
  const eventId = getRouterParam(event, 'id')!
  const l = findList(eventId, getRouterParam(event, 'listId')!)
  const b = validateListInput(await readBody<ListInput>(event), false)
  Object.assign(l, b)
  // Turning contact collection off erases stored contacts (as the server does).
  if (!b.collect_contact) for (const g of guests.filter(x => x.list_id === l.id)) Object.assign(g, { email: '', phone: '' })
  return listView(eventId, l.id)
})
