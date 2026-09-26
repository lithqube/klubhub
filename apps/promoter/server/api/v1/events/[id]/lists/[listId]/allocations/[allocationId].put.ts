import type { AllocationInput } from '~/types/guest'
import { allocationView, findAllocation, findList, validateAllocationInput } from '../../../../../-mockDb'
export default defineEventHandler(async (event) => {
  const l = findList(getRouterParam(event, 'id')!, getRouterParam(event, 'listId')!)
  const a = findAllocation(l.id, getRouterParam(event, 'allocationId')!)
  const b = validateAllocationInput(await readBody<AllocationInput>(event))
  if (b.quota < allocationView(a).used) {
    throw createError({ statusCode: 422, data: { error: 'invalid', field: 'quota', problem: 'lower than the guests already on it; decline or move some first' } })
  }
  Object.assign(a, { ...b, submitter_contact: b.submitter_contact == null ? a.submitter_contact : b.submitter_contact.trim() })
  return allocationView(a)
})
