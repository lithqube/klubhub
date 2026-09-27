import { deleteGuest } from '../../../-mockDb'
export default defineEventHandler((event) => {
  deleteGuest(getRouterParam(event, 'id')!, getRouterParam(event, 'guestId')!)
  setResponseStatus(event, 204)
  return null
})
