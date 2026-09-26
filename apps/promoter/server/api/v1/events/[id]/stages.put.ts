import type { StageInput } from '~/types/event'
import { assertVersion, findEvent, newId, recheck } from '../../-mockDb'
export default defineEventHandler(async (event) => {
  const e = findEvent(getRouterParam(event, 'id')!)
  const b = await readBody<{ version: number, stages: StageInput[] }>(event)
  assertVersion(e, b.version)
  e.stages = b.stages.map((s, i) => ({ id: s.id ?? newId('s'), name: s.name, position: i, curfew_at: s.curfew_at, changeover_minutes: s.changeover_minutes }))
  const ids = new Set(e.stages.map(s => s.id))
  e.lineup = e.lineup.map(l => (l.stage_id && !ids.has(l.stage_id) ? { ...l, stage_id: null } : l))
  return recheck(e)
})
