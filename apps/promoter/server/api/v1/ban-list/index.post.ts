import { addBan } from '../-mockSealed'
// POST /api/v1/ban-list (guestlist.write): {id (client uuid), key_version (active), entry_sealed, expires_at}.
export default defineEventHandler(async (event) => {
  const r = addBan(event, await readBody(event))
  setResponseStatus(event, 201)
  return r
})
