import type { LineupInput } from '~/types/event'
import { assertVersion, findEvent, newId, recheck } from '../../-mockDb'
export default defineEventHandler(async (event) => {
  const e = findEvent(getRouterParam(event, 'id')!)
  const b = await readBody<{ version: number, lineup: LineupInput[] }>(event)
  assertVersion(e, b.version)
  e.lineup = b.lineup.map((l, i) => ({ ...l, id: l.id ?? newId('l'), billing_order: i }))
  return recheck(e)
})
