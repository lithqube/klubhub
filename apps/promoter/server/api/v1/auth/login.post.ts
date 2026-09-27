import { mockSession } from '../-mockDb'

// Mock: accepts any credentials (frontend-only development); a sign-in is the step-up for ERASE NOW.
export default defineEventHandler(() => {
  mockSession.authAt = Date.now()
  return { totp_enrolled: false }
})
