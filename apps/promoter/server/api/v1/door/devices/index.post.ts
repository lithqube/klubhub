import { registerDoorDevice } from '../../-mockDb'
export default defineEventHandler(async (event) => {
  // P2.6: an optional public_key (X25519, base64url) made in the door browser.
  const b = await readBody<{ label: string, public_key?: string }>(event)
  const d = registerDoorDevice(b?.label, b?.public_key)
  setResponseStatus(event, 201)
  return d
})
