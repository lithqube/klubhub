import { findAllocation, findList } from '../../../../../-mockDb'
export default defineEventHandler((event) => {
  const l = findList(getRouterParam(event, 'id')!, getRouterParam(event, 'listId')!)
  const a = findAllocation(l.id, getRouterParam(event, 'allocationId')!)
  a.revoked_at ??= new Date().toISOString()
  setResponseStatus(event, 204)
  return null
})
