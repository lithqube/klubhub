import { DOOR_COOKIE, doorSession, doorSync } from '../-mockDb'
export default defineEventHandler(async (event) => {
  const s = doorSession(getCookie(event, DOOR_COOKIE))
  return doorSync(s, await readBody(event))
})
