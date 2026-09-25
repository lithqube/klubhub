// Mock for frontend-only development: a signed-in owner without TOTP.
export default defineEventHandler(() => ({
  sub: 'local:0190f1d2-7c1a-7a00-9f00-0000000000aa',
  org_id: '0190f1d2-7c1a-7a00-9f00-000000000001',
  roles: ['owner'],
  mfa: false,
  auth_time: new Date().toISOString(),
}))
