import { DOOR_COOKIE, doorBundle, doorSession } from '../-mockDb'
export default defineEventHandler(event => doorBundle(doorSession(getCookie(event, DOOR_COOKIE))))
