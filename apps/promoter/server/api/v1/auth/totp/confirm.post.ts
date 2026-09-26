import { mockSession } from '../../-mockDb'
export default defineEventHandler((event) => {
  mockSession.mfa = true
  setResponseStatus(event, 204)
  return null
})
