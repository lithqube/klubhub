import type { ListInput } from '~/types/guest'
import { newId, standingLists, validateListInput } from '../-mockDb'
export default defineEventHandler(async (event) => {
  const b = validateListInput(await readBody<ListInput>(event), true)
  const s = { ...b, id: newId('sl'), position: Math.max(-1, ...standingLists.map(x => x.position)) + 1 }
  standingLists.push(s)
  setResponseStatus(event, 201)
  return s
})
