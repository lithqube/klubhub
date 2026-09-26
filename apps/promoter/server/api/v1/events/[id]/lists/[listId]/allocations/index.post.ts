import type { AllocationInput } from '~/types/guest'
import { allocationView, allocations, findList, newId, validateAllocationInput } from '../../../../../-mockDb'
export default defineEventHandler(async (event) => {
  const eventId = getRouterParam(event, 'id')!
  const l = findList(eventId, getRouterParam(event, 'listId')!)
  const b = validateAllocationInput(await readBody<AllocationInput>(event))
  const a = { ...b, submitter_contact: b.submitter_contact?.trim() ?? '', id: newId('al'), event_id: eventId, list_id: l.id, revoked_at: null }
  allocations.push(a)
  setResponseStatus(event, 201)
  return allocationView(a)
})
