// Mock: a fixed, obviously fake secret for UI development.
export default defineEventHandler(() => ({
  otpauth_uri: 'otpauth://totp/KlubHub%20Promoter:owner%40nachtwerk.example?secret=JBSWY3DPEHPK3PXP&issuer=KlubHub+Promoter&algorithm=SHA1&digits=6&period=30',
}))
