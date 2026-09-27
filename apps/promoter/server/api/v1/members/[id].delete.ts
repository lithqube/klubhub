import { removeMember } from '../-mockSealed'
// DELETE /api/v1/members/{userID} (member.manage): drops the member's key and wraps, sets rotation_pending.
export default defineEventHandler((event) => {
  removeMember(event, getRouterParam(event, 'id')!)
  setResponseStatus(event, 204)
  return null
})
