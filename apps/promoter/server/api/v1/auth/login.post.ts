import { MOCK_AUTH_COOKIE, MOCK_MFA_COOKIE, mockSession } from '../-mockDb'

// Mock: accepts any credentials (frontend-only development); a sign-in is the step-up for ERASE NOW.
// A sign-in with an authenticator code counts as a second factor for this browser (kh_mock_mfa).
export default defineEventHandler(async (event) => {
  const b = await readBody<{ totp?: string } | null>(event).catch(() => null)
  mockSession.authAt = Date.now()
  setCookie(event, MOCK_AUTH_COOKIE, String(mockSession.authAt), { path: '/', sameSite: 'lax', httpOnly: true })
  if (b?.totp && /^\d{6}$/.test(b.totp)) setCookie(event, MOCK_MFA_COOKIE, '1', { path: '/', sameSite: 'lax', httpOnly: true })
  return { totp_enrolled: false }
})
