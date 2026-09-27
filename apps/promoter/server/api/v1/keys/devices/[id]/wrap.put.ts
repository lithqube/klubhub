import { putDeviceWrap } from '../../../-mockSealed'
// PUT /api/v1/keys/devices/{deviceID}/wrap (door.device.manage): provision a door device.
export default defineEventHandler(async (event) => {
  putDeviceWrap(event, getRouterParam(event, 'id')!, await readBody(event))
  setResponseStatus(event, 204)
  return null
})
