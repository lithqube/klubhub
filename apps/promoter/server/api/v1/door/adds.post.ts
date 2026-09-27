import { DOOR_COOKIE, doorAdds, doorSession } from '../-mockDb'
export default defineEventHandler(async (event) => {
  const s = doorSession(getCookie(event, DOOR_COOKIE))
  return doorAdds(s, await readBody(event))
})
