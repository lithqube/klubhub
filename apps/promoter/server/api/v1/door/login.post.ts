// Mock door login: any 6-digit PIN except '000000' (wrong), '111111' (429 pin_locked) and '999999' (401 pin_expired), for a registered device (see -mockDb.ts).
import { DOOR_COOKIE, doorLogin } from '../-mockDb'
export default defineEventHandler(async (event) => {
  const s = doorLogin(await readBody(event))
  setCookie(event, DOOR_COOKIE, s.session, { httpOnly: true, sameSite: 'lax', path: '/', expires: new Date(s.expires_at) })
  setResponseStatus(event, 204)
  return null
})
