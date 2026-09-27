import { DOOR_COOKIE, doorBundle, doorSession } from '../-mockDb'
import { sealedForDevice } from '../-mockSealed'

// P2.6: the bundle carries the ban list sealed to this device (null when not provisioned).
export default defineEventHandler((event) => {
  const s = doorSession(getCookie(event, DOOR_COOKIE))
  return { ...doorBundle(s), sealed: sealedForDevice(event, s.device_id) }
})
