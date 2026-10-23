import { matchSlots, send } from '../-mockDb'

export default defineEventHandler((event) => {
  const slotId = getQuery(event).slot_id
  return send(event, matchSlots(slotId ? String(slotId) : undefined))
})
