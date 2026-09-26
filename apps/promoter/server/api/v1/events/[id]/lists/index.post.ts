import type { ListInput } from '~/types/guest'
import { findEvent, guestLists, listView, newId, validateListInput } from '../../../-mockDb'
export default defineEventHandler(async (event) => {
  const e = findEvent(getRouterParam(event, 'id')!)
  const b = validateListInput(await readBody<ListInput>(event), false)
  const id = newId('gl')
  const position = Math.max(-1, ...guestLists.filter(l => l.event_id === e.id).map(l => l.position)) + 1
  guestLists.push({ ...b, id, event_id: e.id, standing_template_id: null, position })
  setResponseStatus(event, 201)
  return listView(e.id, id)
})
