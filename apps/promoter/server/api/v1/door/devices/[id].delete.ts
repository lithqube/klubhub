import { revokeDoorDevice } from '../../-mockDb'
import { onDeviceRevoked } from '../../-mockSealed'
export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')!
  revokeDoorDevice(id)
  // P2.6: its wraps are deleted and the sealed key must rotate.
  onDeviceRevoked(event, id)
  setResponseStatus(event, 204)
  return null
})
