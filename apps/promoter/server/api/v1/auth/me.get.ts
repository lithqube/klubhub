// Mock for frontend-only development: a signed-in owner, TOTP off until confirmed.
import { mockSession } from '../-mockDb'
export default defineEventHandler(() => ({
  sub: 'local:0190f1d2-7c1a-7a00-9f00-0000000000aa',
  org_id: '0190f1d2-7c1a-7a00-9f00-000000000001',
  roles: ['owner'],
  mfa: mockSession.mfa,
  auth_time: new Date().toISOString(),
}))
