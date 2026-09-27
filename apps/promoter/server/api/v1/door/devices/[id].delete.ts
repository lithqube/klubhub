import { revokeDoorDevice } from '../../-mockDb'
export default defineEventHandler((event) => {
  revokeDoorDevice(getRouterParam(event, 'id')!)
  setResponseStatus(event, 204)
  return null
})
