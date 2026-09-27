// Sealed tier mock (P2.6): member keys, org sealed key versions, wraps, the
// rotation flag and the ban list. Like the server, it stores ciphertext and
// public keys only and never decrypts; it checks shapes, lengths, versions
// and the security.manage policy (owner, second factor, recent sign-in).
//
// State is kept per "sealed namespace": the kh_mock_sealed cookie (e2e
// specs set their own so parallel runs never see each other's keys, and
// "409 already set up" stays testable), else one shared default namespace
// for manual development. Nothing is seeded sealed.
import { createHash, generateKeyPairSync } from 'node:crypto'
import type {
  BanInput, BanListResponse, BanRecord, DoorSealed, KdfParams, MemberKey, OrgKeyInfo, OrgSetupInput, Recipients, RecoveryWrap, RotateInput,
  WrapInput,
} from '~/types/sealed'
import { doorDevices, mockAuthAt, mockMfa } from './-mockDb'

type H3Event = Parameters<typeof getCookie>[0]

export const SEALED_NS_COOKIE = 'kh_mock_sealed'
const STEP_UP_MS = 15 * 60_000
const DAY = 86_400_000
/** The mock session's user (auth/me.get.ts): sub "local:<uuid>". */
export const MOCK_USER_ID = '0190f1d2-7c1a-7a00-9f00-0000000000aa'

interface Member { user_id: string, name: string, email: string, role: string }
interface OrgKey { version: number, status: 'active' | 'retired', recovery_public_key: string, recovery_fingerprint: string, created_at: string, retired_at: string | null }
interface WrapRow { version: number, recipient_kind: 'member' | 'device' | 'recovery', recipient_id: string | null, wrap_sealed: string, created_at: string }
interface BanRow extends BanRecord { created_by: string }

interface Ns {
  members: Member[]
  memberKeys: Map<string, MemberKey & { created_at: string, updated_at: string }>
  orgKeys: OrgKey[]
  wraps: WrapRow[]
  state: { rotation_pending: boolean, rotation_reason: string | null, pending_since: string | null }
  ban: BanRow[]
}

/** A real X25519 public key for a fictional member (so owners can grant to her). */
function demoPublicKey(): string {
  const { publicKey } = generateKeyPairSync('x25519')
  return publicKey.export({ format: 'jwk' }).x as string
}

function freshNs(): Ns {
  const now = new Date().toISOString()
  const ns: Ns = {
    // All names are fictional.
    members: [
      { user_id: MOCK_USER_ID, name: 'Owner (you)', email: 'owner@nachtwerk.example', role: 'owner' },
      { user_id: '0190f1d2-7c1a-7a00-9f00-0000000000b1', name: 'Lena Park', email: 'lena@nachtwerk.example', role: 'booker' },
      { user_id: '0190f1d2-7c1a-7a00-9f00-0000000000b2', name: 'Sam Ortiz', email: 'sam@nachtwerk.example', role: 'admin' },
    ],
    memberKeys: new Map(),
    orgKeys: [],
    wraps: [],
    state: { rotation_pending: false, rotation_reason: null, pending_since: null },
    ban: [],
  }
  // Lena already made her member key (she has no wrap until an owner grants one).
  ns.memberKeys.set(ns.members[1]!.user_id, {
    public_key: demoPublicKey(), private_sealed: Buffer.alloc(61, 1).toString('base64url'),
    kdf: { alg: 'argon2id', m: 65_536, t: 3, p: 1, salt: Buffer.alloc(16, 2).toString('base64url') }, created_at: now, updated_at: now,
  })
  return ns
}

const spaces = new Map<string, Ns>()

function nsOf(event: H3Event): Ns {
  const raw = getCookie(event, SEALED_NS_COOKIE)
  const key = raw && /^[\w-]{1,64}$/.test(raw) ? raw : 'default'
  let ns = spaces.get(key)
  if (!ns) spaces.set(key, (ns = freshNs()))
  return ns
}

const err = (statusCode: number, error: string, extra: Record<string, unknown> = {}) => createError({ statusCode, data: { error, ...extra } })
const invalid = (field: string, problem: string) => err(422, 'invalid', { field, problem })

/** security.manage: owner (the mock user is one), a second factor, a sign-in in the last 15 minutes. */
function securityManage(event: H3Event) {
  if (!mockMfa(event)) throw err(403, 'mfa_required')
  if (Date.now() - mockAuthAt(event) > STEP_UP_MS) throw err(403, 'reauthentication_required')
}

function bytes(v: unknown, field: string, len?: { eq?: number, min?: number, v1?: boolean }): string {
  if (typeof v !== 'string' || !/^[\w-]*={0,2}$/.test(v)) throw invalid(field, 'base64url')
  const b = Buffer.from(v, 'base64url')
  if (len?.eq !== undefined && b.length !== len.eq) throw invalid(field, `${len.eq} bytes`)
  if (len?.min !== undefined && b.length < len.min) throw invalid(field, `at least ${len.min} bytes`)
  if (len?.v1 && b[0] !== 1) throw invalid(field, 'version byte 0x01')
  return v
}
/** 0x01 ‖ eph_pub(32) ‖ nonce(12) ‖ ct+tag with a 32-byte OSK inside. */
const wrapBytes = (v: unknown, field: string) => bytes(v, field, { eq: 1 + 32 + 12 + 32 + 16, v1: true })
/** 0x01 ‖ nonce(12) ‖ ct+tag. */
const aesBytes = (v: unknown, field: string) => bytes(v, field, { min: 1 + 12 + 16 + 2, v1: true })

const active = (ns: Ns) => ns.orgKeys.find(k => k.status === 'active') ?? null

function fp(pub: string): string {
  return createHash('sha256').update(Buffer.from(pub, 'base64url')).digest('hex').slice(0, 16)
}

// ---------------------------------------------------------------- /keys/me

export function getMemberKey(event: H3Event): MemberKey {
  const k = nsOf(event).memberKeys.get(MOCK_USER_ID)
  if (!k) throw err(404, 'no_member_key')
  return { public_key: k.public_key, private_sealed: k.private_sealed, kdf: k.kdf }
}

function validKdf(k: Partial<KdfParams> | undefined): KdfParams {
  const int = (n: unknown, lo: number, hi: number) => typeof n === 'number' && Number.isInteger(n) && n >= lo && n <= hi
  // Same bounds as the API: m 19 MiB to 256 MiB, t 1 to 10, p 1 to 4.
  if (!k || k.alg !== 'argon2id' || !int(k.m, 19 * 1024, 262_144) || !int(k.t, 1, 10) || !int(k.p, 1, 4)) throw invalid('kdf', 'argon2id with m, t, p')
  bytes(k.salt, 'kdf.salt', { eq: 16 })
  return { alg: 'argon2id', m: k.m!, t: k.t!, p: k.p!, salt: k.salt! }
}

/**
 * Create the key (201), or re-wrap it (same public key, 200). A different
 * public key replaces the key only for security.manage (an owner
 * recovering with the kit; the old key's wraps are dropped), else 409
 * public_key_mismatch — as the API.
 */
export function putMemberKey(event: H3Event, b: Partial<MemberKey>): { key: MemberKey, created: boolean } {
  const ns = nsOf(event)
  const pub = bytes(b?.public_key, 'public_key', { eq: 32 })
  const priv = bytes(b?.private_sealed, 'private_sealed', { min: 1 + 12 + 32 + 16, v1: true })
  const kdf = validKdf(b?.kdf)
  const now = new Date().toISOString()
  const old = ns.memberKeys.get(MOCK_USER_ID)
  if (old && old.public_key !== pub) {
    try {
      securityManage(event)
    } catch {
      throw err(409, 'public_key_mismatch')
    }
    ns.wraps = ns.wraps.filter(w => !(w.recipient_kind === 'member' && w.recipient_id === MOCK_USER_ID))
  }
  ns.memberKeys.set(MOCK_USER_ID, { public_key: pub, private_sealed: priv, kdf, created_at: old && old.public_key === pub ? old.created_at : now, updated_at: now })
  return { key: { public_key: pub, private_sealed: priv, kdf }, created: !old }
}

// ---------------------------------------------------------------- /keys/org

export function orgKeyInfo(event: H3Event): OrgKeyInfo {
  const ns = nsOf(event)
  const a = active(ns)
  if (!a) return { status: 'not_setup', version: null, my_wrap: null, recovery_fingerprint: null }
  const mine = ns.wraps.find(w => w.version === a.version && w.recipient_kind === 'member' && w.recipient_id === MOCK_USER_ID)
  return { status: ns.state.rotation_pending ? 'rotation_pending' : 'ready', version: a.version, my_wrap: mine?.wrap_sealed ?? null, recovery_fingerprint: a.recovery_fingerprint }
}

export function recipients(event: H3Event): Recipients {
  securityManage(event)
  const ns = nsOf(event)
  const a = active(ns)
  const has = (kind: 'member' | 'device', id: string) => !!a && ns.wraps.some(w => w.version === a.version && w.recipient_kind === kind && w.recipient_id === id)
  return {
    version: a?.version ?? null,
    members: ns.members.map(m => ({ ...m, public_key: ns.memberKeys.get(m.user_id)?.public_key ?? null, has_wrap: has('member', m.user_id) })),
    devices: doorDevices.map(d => ({ device_id: d.id, label: d.label, public_key: d.public_key, has_wrap: has('device', d.id), revoked: !!d.revoked_at })),
  }
}

function checkWraps(ns: Ns, wraps: unknown): WrapInput[] {
  if (!Array.isArray(wraps) || wraps.length > 500) throw invalid('wraps', 'a list of wraps')
  return wraps.map((w: Partial<WrapInput>, i) => {
    const f = `wraps[${i}]`
    if (w?.recipient_kind === 'member') {
      if (!ns.members.some(m => m.user_id === w.recipient_id)) throw err(422, 'unknown_recipient', { field: f })
      if (!ns.memberKeys.has(w.recipient_id!)) throw err(422, 'no_public_key', { field: f })
    } else if (w?.recipient_kind === 'device') {
      const d = doorDevices.find(x => x.id === w.recipient_id)
      if (!d || d.revoked_at) throw err(422, 'unknown_recipient', { field: f })
      if (!d.public_key) throw err(422, 'no_public_key', { field: f })
    } else {
      throw invalid(`${f}.recipient_kind`, 'member or device')
    }
    return { recipient_kind: w.recipient_kind, recipient_id: w.recipient_id!, wrap: wrapBytes(w.wrap, `${f}.wrap`) }
  })
}

function upsertWraps(ns: Ns, version: number, wraps: WrapInput[]) {
  const now = new Date().toISOString()
  for (const w of wraps) {
    ns.wraps = ns.wraps.filter(x => !(x.version === version && x.recipient_kind === w.recipient_kind && x.recipient_id === w.recipient_id))
    ns.wraps.push({ version, recipient_kind: w.recipient_kind, recipient_id: w.recipient_id, wrap_sealed: w.wrap, created_at: now })
  }
}

function checkRecovery(r: unknown) {
  const x = r as Partial<RecoveryWrap> | undefined
  const pub = bytes(x?.public_key, 'recovery.public_key', { eq: 32 })
  if (typeof x?.fingerprint !== 'string' || x.fingerprint.toLowerCase() !== fp(pub)) throw invalid('recovery.fingerprint', 'first 8 bytes of SHA-256(public key), hex')
  return { public_key: pub, fingerprint: x.fingerprint.toLowerCase(), wrap: wrapBytes(x.wrap, 'recovery.wrap') }
}

export function setupOrg(event: H3Event, b: Partial<OrgSetupInput>) {
  securityManage(event)
  const ns = nsOf(event)
  if (ns.orgKeys.length) throw err(409, 'already_setup')
  if (b?.version !== 1) throw invalid('version', 'must be 1')
  const rec = checkRecovery(b.recovery)
  const wraps = checkWraps(ns, b.wraps)
  if (!wraps.some(w => w.recipient_kind === 'member' && w.recipient_id === MOCK_USER_ID)) throw invalid('wraps', 'must include a wrap for you')
  const now = new Date().toISOString()
  ns.orgKeys.push({ version: 1, status: 'active', recovery_public_key: rec.public_key, recovery_fingerprint: rec.fingerprint, created_at: now, retired_at: null })
  ns.wraps.push({ version: 1, recipient_kind: 'recovery', recipient_id: null, wrap_sealed: rec.wrap, created_at: now })
  upsertWraps(ns, 1, wraps)
  ns.state = { rotation_pending: false, rotation_reason: null, pending_since: null }
}

export function addWraps(event: H3Event, b: { version?: number, wraps?: unknown }) {
  securityManage(event)
  const ns = nsOf(event)
  const a = active(ns)
  if (!a) throw err(409, 'not_setup')
  if (b?.version !== undefined && b.version !== a.version) throw err(409, 'version_conflict')
  upsertWraps(ns, a.version, checkWraps(ns, b.wraps))
}

export function rotate(event: H3Event, b: Partial<RotateInput>) {
  securityManage(event)
  const ns = nsOf(event)
  const a = active(ns)
  if (!a) throw err(409, 'not_setup')
  if (b?.from_version !== a.version) throw err(409, 'version_conflict')
  if (b.to_version !== a.version + 1) throw invalid('to_version', 'from_version + 1')
  const rec = checkRecovery(b.recovery)
  const wraps = checkWraps(ns, b.wraps)
  if (!wraps.some(w => w.recipient_kind === 'member' && w.recipient_id === MOCK_USER_ID)) throw invalid('wraps', 'must include a wrap for you')
  const live = ns.ban.filter(e => Date.parse(e.expires_at) > Date.now())
  const given = new Map((Array.isArray(b.ban_entries) ? b.ban_entries : []).map(e => [e?.id, aesBytes(e?.entry_sealed, 'ban_entries.entry_sealed')]))
  if (live.some(e => !given.has(e.id)) || given.size !== live.length) throw invalid('ban_entries', 'must cover every ban entry exactly')
  const now = new Date().toISOString()
  Object.assign(a, { status: 'retired', retired_at: now })
  const to = b.to_version
  ns.orgKeys.push({ version: to, status: 'active', recovery_public_key: rec.public_key, recovery_fingerprint: rec.fingerprint, created_at: now, retired_at: null })
  ns.wraps.push({ version: to, recipient_kind: 'recovery', recipient_id: null, wrap_sealed: rec.wrap, created_at: now })
  upsertWraps(ns, to, wraps)
  ns.ban = ns.ban.filter(e => given.has(e.id)).map(e => ({ ...e, key_version: to, entry_sealed: given.get(e.id)!, updated_at: now }))
  ns.state = { rotation_pending: false, rotation_reason: null, pending_since: null }
}

export function recoveryWrap(event: H3Event): RecoveryWrap {
  securityManage(event)
  const ns = nsOf(event)
  const a = active(ns)
  if (!a) throw err(404, 'not_setup')
  const w = ns.wraps.find(x => x.version === a.version && x.recipient_kind === 'recovery')!
  return { version: a.version, public_key: a.recovery_public_key, fingerprint: a.recovery_fingerprint, wrap: w.wrap_sealed }
}

/** PUT /keys/devices/{id}/wrap (door.device.manage: no second factor needed). */
export function putDeviceWrap(event: H3Event, deviceId: string, b: { version?: number, wrap?: unknown }) {
  const ns = nsOf(event)
  const a = active(ns)
  if (!a) throw err(409, 'not_setup')
  const d = doorDevices.find(x => x.id === deviceId)
  if (!d || d.revoked_at) throw err(404, 'not_found')
  if (!d.public_key) throw err(422, 'no_public_key')
  if (b?.version !== undefined && b.version !== a.version) throw err(409, 'version_conflict')
  upsertWraps(ns, a.version, [{ recipient_kind: 'device', recipient_id: deviceId, wrap: wrapBytes(b.wrap, 'wrap') }])
}

/** A revoked door device loses its wraps everywhere; this collective must rotate. */
export function onDeviceRevoked(event: H3Event, deviceId: string) {
  for (const ns of spaces.values()) ns.wraps = ns.wraps.filter(w => !(w.recipient_kind === 'device' && w.recipient_id === deviceId))
  const ns = nsOf(event)
  if (active(ns)) ns.state = { rotation_pending: true, rotation_reason: 'device_revoked', pending_since: new Date().toISOString() }
}

/** DELETE /members/{userID} (member.manage: second factor + recent sign-in). */
export function removeMember(event: H3Event, userId: string) {
  if (!mockMfa(event)) throw err(403, 'mfa_required')
  if (Date.now() - mockAuthAt(event) > STEP_UP_MS) throw err(403, 'reauthentication_required')
  const ns = nsOf(event)
  const m = ns.members.find(x => x.user_id === userId)
  if (!m) throw err(404, 'not_found')
  if (userId === MOCK_USER_ID) throw err(409, 'cannot_remove_self')
  if (m.role === 'owner' && ns.members.filter(x => x.role === 'owner').length < 2) throw err(409, 'last_owner')
  ns.members = ns.members.filter(x => x.user_id !== userId)
  ns.memberKeys.delete(userId)
  ns.wraps = ns.wraps.filter(w => !(w.recipient_kind === 'member' && w.recipient_id === userId))
  if (active(ns)) ns.state = { rotation_pending: true, rotation_reason: 'member_removed', pending_since: new Date().toISOString() }
}

// ---------------------------------------------------------------- ban list

const liveBan = (ns: Ns) => ns.ban.filter(e => Date.parse(e.expires_at) > Date.now())
const view = ({ created_by: _c, ...e }: BanRow): BanRecord => e

export function banList(event: H3Event): BanListResponse {
  const ns = nsOf(event)
  return { key_version: active(ns)?.version ?? null, entries: liveBan(ns).map(view) }
}

function checkBan(ns: Ns, b: Partial<BanInput>) {
  const a = active(ns)
  if (!a) throw err(409, 'not_setup')
  if (b?.key_version !== a.version) throw err(409, 'key_version_stale')
  const sealed = aesBytes(b.entry_sealed, 'entry_sealed')
  const exp = Date.parse(b.expires_at ?? '')
  const now = Date.now()
  if (!Number.isFinite(exp) || exp < now + DAY || exp > now + 3 * 366 * DAY) throw invalid('expires_at', 'between now + 1 day and now + 3 years')
  return { key_version: a.version, entry_sealed: sealed, expires_at: new Date(exp).toISOString() }
}

export function addBan(event: H3Event, b: Partial<BanInput>): BanRecord {
  const ns = nsOf(event)
  if (typeof b?.id !== 'string' || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(b.id)) throw invalid('id', 'a client uuid')
  const c = checkBan(ns, b)
  if (ns.ban.some(e => e.id === b.id)) throw err(409, 'ban_entry_exists')
  const now = new Date().toISOString()
  const row: BanRow = { id: b.id, ...c, created_at: now, updated_at: now, created_by: `local:${MOCK_USER_ID}` }
  ns.ban.push(row)
  return view(row)
}

export function updateBan(event: H3Event, id: string, b: Partial<BanInput>): BanRecord {
  const ns = nsOf(event)
  const row = liveBan(ns).find(e => e.id === id)
  if (!row) throw err(404, 'not_found')
  Object.assign(row, checkBan(ns, b), { updated_at: new Date().toISOString() })
  return view(row)
}

export function deleteBan(event: H3Event, id: string) {
  const ns = nsOf(event)
  if (!ns.ban.some(e => e.id === id)) throw err(404, 'not_found')
  ns.ban = ns.ban.filter(e => e.id !== id)
}

// ---------------------------------------------------------------- door bundle

/** GET /door/devices additions (as the API): the device's public key and whether it holds the active wrap. */
export function deviceSealedFields(event: H3Event, deviceId: string): { public_key: string | null, has_wrap: boolean } {
  const ns = nsOf(event)
  const a = active(ns)
  const d = doorDevices.find(x => x.id === deviceId)
  return { public_key: d?.public_key ?? null, has_wrap: !!a && ns.wraps.some(w => w.version === a.version && w.recipient_kind === 'device' && w.recipient_id === deviceId) }
}

/** The bundle's sealed block for a device: null unless set up and this device holds the active wrap. */
export function sealedForDevice(event: H3Event, deviceId: string): DoorSealed | null {
  const ns = nsOf(event)
  const a = active(ns)
  const w = a && ns.wraps.find(x => x.version === a.version && x.recipient_kind === 'device' && x.recipient_id === deviceId)
  if (!a || !w) return null
  return { key_version: a.version, wrap: w.wrap_sealed, ban_entries: liveBan(ns).map(e => ({ id: e.id, entry_sealed: e.entry_sealed, expires_at: e.expires_at })) }
}
