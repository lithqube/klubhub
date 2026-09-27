// Mirrors the P2.6 contract (.claude/plans/promoter-p2-guests-door.plan.md,
// "P2.6 contract (sealed tier and ban list)"). Every byte field is
// base64url (no padding) in JSON; times are RFC 3339 UTC.

/** Argon2id parameters stored with a member key; m in KiB, salt base64url (16 bytes). */
export interface KdfParams {
  alg: 'argon2id'
  m: number
  t: number
  p: number
  salt: string
}

/** GET/PUT /api/v1/keys/me */
export interface MemberKey {
  public_key: string
  /** 0x01 ‖ nonce ‖ AES-GCM(private key) under the Argon2id key. */
  private_sealed: string
  kdf: KdfParams
}

export type OrgKeyStatus = 'not_setup' | 'ready' | 'rotation_pending'

/** GET /api/v1/keys/org */
export interface OrgKeyInfo {
  status: OrgKeyStatus
  version: number | null
  /** The caller's wrap of the active OSK, or null. */
  my_wrap: string | null
  recovery_fingerprint: string | null
}

export type RecipientKind = 'member' | 'device' | 'recovery'

export interface WrapInput {
  recipient_kind: 'member' | 'device'
  recipient_id: string
  wrap: string
}

export interface RecoveryInput {
  public_key: string
  /** First 8 bytes of SHA-256(public key), hex. */
  fingerprint: string
  wrap: string
}

/** POST /api/v1/keys/org/setup */
export interface OrgSetupInput {
  version: 1
  recovery: RecoveryInput
  wraps: WrapInput[]
}

/** POST /api/v1/keys/org/wraps */
export interface WrapsInput {
  version: number
  wraps: WrapInput[]
}

/** POST /api/v1/keys/org/rotate */
export interface RotateInput {
  from_version: number
  to_version: number
  recovery: RecoveryInput
  wraps: WrapInput[]
  ban_entries: { id: string, entry_sealed: string }[]
}

export interface MemberRecipient {
  user_id: string
  name: string
  email: string
  role: string
  public_key: string | null
  has_wrap: boolean
}

export interface DeviceRecipient {
  device_id: string
  label: string
  public_key: string | null
  has_wrap: boolean
  revoked: boolean
}

/** GET /api/v1/keys/org/recipients */
export interface Recipients {
  version?: number | null
  members: MemberRecipient[]
  devices: DeviceRecipient[]
}

/** GET /api/v1/keys/org/recovery */
export interface RecoveryWrap {
  version?: number
  public_key: string
  fingerprint: string
  wrap: string
}

// ---------------------------------------------------------------- ban list

/** What is encrypted per entry (name and reason required). */
export interface BanPlain {
  name: string
  email?: string
  reason: string
  note?: string
}

/** One entry as the API stores it (ciphertext only). */
export interface BanRecord {
  id: string
  key_version: number
  entry_sealed: string
  expires_at: string
  created_at: string
  updated_at: string
  /** Who added it (the staff member's display name; GET /ban-list only, null when unknown). */
  created_by_name?: string | null
}

/** GET /api/v1/ban-list (expired entries excluded). */
export interface BanListResponse {
  key_version: number | null
  entries: BanRecord[]
}

/** POST /api/v1/ban-list, PUT /api/v1/ban-list/{id} */
export interface BanInput {
  id: string
  key_version: number
  entry_sealed: string
  expires_at: string
}

/** A decrypted entry in memory; `plain` is null when it does not open with the current key. */
export interface BanEntry {
  id: string
  key_version: number
  expires_at: string
  created_at: string
  updated_at: string
  /** Who added it; "You" for an entry added in this tab (the write response carries no name). */
  created_by_name: string | null
  plain: BanPlain | null
}

/** The door bundle's sealed block (null when the org is not set up or this device has no wrap). */
export interface DoorSealed {
  key_version: number
  wrap: string
  ban_entries: { id: string, entry_sealed: string, expires_at: string }[]
}
