// Mock: staff PINs are random; the manager PIN is always '246810' (see -mockDb.ts).
import { setDoorPin } from '../../../-mockDb'
export default defineEventHandler(async (event) => {
  const r = setDoorPin(getRouterParam(event, 'id')!, await readBody(event))
  setResponseStatus(event, 201)
  return r
})
