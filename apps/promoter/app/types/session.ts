export type Role = 'owner' | 'admin' | 'booker' | 'finance' | 'marketing' | 'door'

/** GET /api/v1/auth/me */
export interface Me {
  sub: string
  org_id: string
  roles: Role[]
  mfa: boolean
  auth_time: string
  event_scope?: string
  /** The account's email, if the API includes it (used to label the sealed passphrase for password managers). */
  email?: string
}
