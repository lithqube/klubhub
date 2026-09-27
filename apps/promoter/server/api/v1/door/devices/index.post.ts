import { registerDoorDevice } from '../../-mockDb'
export default defineEventHandler(async (event) => {
  const d = registerDoorDevice((await readBody<{ label: string }>(event))?.label)
  setResponseStatus(event, 201)
  return d
})
