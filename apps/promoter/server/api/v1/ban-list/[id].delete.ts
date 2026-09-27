import { deleteBan } from '../-mockSealed'
// DELETE /api/v1/ban-list/{id} (guestlist.write).
export default defineEventHandler((event) => {
  deleteBan(event, getRouterParam(event, 'id')!)
  setResponseStatus(event, 204)
  return null
})
