import type { EventStatus } from '~/types/event'
import { hasErrors } from '~/utils/timetable'
import { assertVersion, findEvent, recheck } from '../../-mockDb'
const transitions: Record<EventStatus, EventStatus[]> = {
  draft: ['scheduled', 'published', 'cancelled'], scheduled: ['draft', 'published', 'cancelled', 'postponed'],
  published: ['cancelled', 'postponed'], postponed: ['scheduled', 'published', 'cancelled'], cancelled: [],
}
export default defineEventHandler(async (event) => {
  const e = findEvent(getRouterParam(event, 'id')!)
  const b = await readBody<{ version: number, status: EventStatus }>(event)
  assertVersion(e, b.version)
  if (!transitions[e.status].includes(b.status)) throw createError({ statusCode: 409, data: { error: 'bad_transition' } })
  if ((b.status === 'published' || b.status === 'scheduled') && hasErrors(e.issues)) {
    throw createError({ statusCode: 409, data: { error: 'timetable_errors', issues: e.issues } })
  }
  if (b.status === 'scheduled' && !(e.publish_at && Date.parse(e.publish_at) > Date.now())) {
    throw createError({ statusCode: 409, data: { error: 'publish_at_required' } })
  }
  e.status = b.status
  return recheck(e)
})
