import { MOCK_AUTH_COOKIE, mockSession } from '../-mockDb'

// Mock: accepts any credentials (frontend-only development); a sign-in is the step-up for ERASE NOW.
export default defineEventHandler((event) => {
  mockSession.authAt = Date.now()
  setCookie(event, MOCK_AUTH_COOKIE, String(mockSession.authAt), { path: '/', sameSite: 'lax', httpOnly: true })
  return { totp_enrolled: false }
})
