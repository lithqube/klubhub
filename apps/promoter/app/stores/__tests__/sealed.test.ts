import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { BanRecord, KdfParams, MemberKey, OrgSetupInput, RotateInput, WrapInput } from '~/types/sealed'
import { b64url, fromB64url, randomBytes } from '~/utils/sealed/bytes'
import { fingerprint, keyFingerprint, oskAad, unwrapOsk } from '~/utils/sealed/keys'
import { x25519Keypair } from '~/utils/sealed/seal'
import { openDoorBan } from '~/utils/doorBan'
import { useSessionStore } from '../session'
import { IDLE_LOCK_MS, useSealedStore } from '../sealed'
import { useBanListStore } from '../banList'

// Tiny Argon2id parameters for new keys: the construction is what is tested here.
vi.mock('~/utils/sealed/kdf', async (orig) => {
  const real = await orig<typeof import('~/utils/sealed/kdf')>()
  return { ...real, newKdfParams: (): KdfParams => ({ alg: 'argon2id', m: 8, t: 1, p: 1, salt: real.newKdfParams().salt }) }
})

const TENANT = 'org-1'
const SUB = 'local:user-owner'
const ME = 'user-owner'
const PASS = 'purple tiger river moon'

interface Sent { method?: string, body?: unknown, headers?: Record<string, string> }

/** A tiny fake of the P2.6 API: stores ciphertext and public keys only. */
function fakeApi() {
  const db = {
    keys: new Map<string, MemberKey>(),
    version: null as number | null,
    recovery: null as { public_key: string, fingerprint: string } | null,
    wraps: [] as (WrapInput & { version: number })[],
    recoveryWraps: new Map<number, string>(),
    ban: [] as BanRecord[],
    pending: false,
    lena: x25519Keypair(),
    door: x25519Keypair(),
    staleNext: false,
  }
  const err = (statusCode: number, error: string) => Object.assign(new Error(error), { statusCode, data: { error } })
  const myWrap = () => db.wraps.find(w => w.version === db.version && w.recipient_kind === 'member' && w.recipient_id === ME)?.wrap ?? null
  const handler = vi.fn(async (url: string, opts: Sent = {}) => {
    const m = (opts.method ?? 'GET').toUpperCase()
    const body = opts.body as Record<string, unknown>
    if (url === '/api/v1/keys/me') {
      if (m === 'GET') {
        const k = db.keys.get(ME)
        if (!k) throw err(404, 'no_member_key')
        return k
      }
      const old = db.keys.get(ME)
      if (old && old.public_key !== body.public_key) db.wraps = db.wraps.filter(w => !(w.recipient_kind === 'member' && w.recipient_id === ME))
      db.keys.set(ME, body as unknown as MemberKey)
      return null
    }
    if (url === '/api/v1/keys/org') {
      return { status: db.version === null ? 'not_setup' : db.pending ? 'rotation_pending' : 'ready', version: db.version, my_wrap: myWrap(), recovery_fingerprint: db.recovery?.fingerprint ?? null }
    }
    if (url === '/api/v1/keys/org/setup') {
      if (db.version !== null) throw err(409, 'already_setup')
      const b = body as unknown as OrgSetupInput
      db.version = b.version
      db.recovery = { public_key: b.recovery.public_key, fingerprint: b.recovery.fingerprint }
      db.recoveryWraps.set(b.version, b.recovery.wrap)
      db.wraps.push(...b.wraps.map(w => ({ ...w, version: b.version })))
      return null
    }
    if (url === '/api/v1/keys/org/wraps') {
      const b = body as { version: number, wraps: WrapInput[] }
      for (const w of b.wraps) {
        db.wraps = db.wraps.filter(x => !(x.version === b.version && x.recipient_kind === w.recipient_kind && x.recipient_id === w.recipient_id))
        db.wraps.push({ ...w, version: b.version })
      }
      return null
    }
    if (url === '/api/v1/keys/org/recipients') {
      const has = (kind: string, id: string) => db.wraps.some(w => w.version === db.version && w.recipient_kind === kind && w.recipient_id === id)
      return {
        members: [
          { user_id: ME, name: 'Owner', email: 'o@x', role: 'owner', public_key: db.keys.get(ME)?.public_key ?? null, has_wrap: has('member', ME) },
          { user_id: 'user-lena', name: 'Lena', email: 'l@x', role: 'booker', public_key: b64url(db.lena.publicKey), has_wrap: has('member', 'user-lena') },
          { user_id: 'user-sam', name: 'Sam', email: 's@x', role: 'admin', public_key: null, has_wrap: false },
        ],
        devices: [{ device_id: 'dev-1', label: 'Door 1', public_key: b64url(db.door.publicKey), has_wrap: has('device', 'dev-1'), revoked: false }],
      }
    }
    if (url === '/api/v1/keys/org/recovery') return { version: db.version, ...db.recovery, wrap: db.recoveryWraps.get(db.version!) }
    if (url === '/api/v1/keys/org/rotate') {
      const b = body as unknown as RotateInput
      if (b.from_version !== db.version) throw err(409, 'version_conflict')
      const ids = new Set(b.ban_entries.map(e => e.id))
      if (db.ban.some(e => !ids.has(e.id))) throw err(422, 'invalid')
      db.version = b.to_version
      db.recoveryWraps.set(b.to_version, b.recovery.wrap)
      db.wraps.push(...b.wraps.map(w => ({ ...w, version: b.to_version })))
      db.ban = db.ban.map(e => ({ ...e, key_version: b.to_version, entry_sealed: b.ban_entries.find(x => x.id === e.id)!.entry_sealed }))
      db.pending = false
      return null
    }
    if (url.startsWith('/api/v1/keys/devices/')) {
      const b = body as { version: number, wrap: string }
      db.wraps.push({ recipient_kind: 'device', recipient_id: url.split('/')[5]!, wrap: b.wrap, version: b.version })
      return null
    }
    if (url === '/api/v1/ban-list') {
      if (m === 'GET') return { key_version: db.version, entries: db.ban }
      const b = body as unknown as BanRecord
      if (db.staleNext) {
        db.staleNext = false
        throw err(409, 'key_version_stale')
      }
      if (b.key_version !== db.version) throw err(409, 'key_version_stale')
      const now = new Date().toISOString()
      db.ban.push({ id: b.id, key_version: b.key_version, entry_sealed: b.entry_sealed, expires_at: b.expires_at, created_at: now, updated_at: now })
      return db.ban.at(-1)
    }
    if (url.startsWith('/api/v1/ban-list/')) {
      const id = url.split('/').pop()!
      const e = db.ban.find(x => x.id === id)
      if (!e) throw err(404, 'not_found')
      if (m === 'DELETE') {
        db.ban = db.ban.filter(x => x.id !== id)
        return null
      }
      const b = body as unknown as BanRecord
      if (b.key_version !== db.version) throw err(409, 'key_version_stale')
      Object.assign(e, { entry_sealed: b.entry_sealed, key_version: b.key_version, expires_at: b.expires_at, updated_at: new Date().toISOString() })
      return e
    }
    throw err(404, 'not_found')
  })
  return { db, handler }
}

let api: ReturnType<typeof fakeApi>

/** The options of the first request to url (with that method). */
function sent<T>(url: string, method?: string): { body: T, headers?: Record<string, string> } {
  const c = api.handler.mock.calls.find(x => x[0] === url && (!method || x[1]?.method === method))
  if (!c?.[1]) throw new Error(`no request to ${url}`)
  return c[1] as { body: T, headers?: Record<string, string> }
}

function signIn(roles: string[] = ['owner']) {
  useSessionStore().me = { sub: SUB, org_id: TENANT, roles: roles as never, mfa: true, auth_time: new Date().toISOString() }
}

async function ownerSetUp() {
  const s = useSealedStore()
  await s.fetchStatus()
  await s.setupMemberKey(PASS)
  const draft = await s.startSetup()
  const [a, b] = draft.challenge
  await s.finishSetup([draft.groups[a]!, draft.groups[b]!])
  return { s, groups: draft.groups }
}

const later = (days: number) => new Date(Date.now() + days * 86_400_000).toISOString()

describe('useSealedStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    api = fakeApi()
    vi.stubGlobal('$fetch', api.handler)
    signIn()
  })
  afterEach(() => {
    useSealedStore().lock()
    vi.useRealTimers()
  })

  it('walks no key → member key → owner setup with the recovery kit → unlocked', async () => {
    const s = useSealedStore()
    await s.fetchStatus()
    expect(s.state).toBe('no_key')
    await expect(s.setupMemberKey('too short')).rejects.toMatchObject({ error: 'passphrase_short' })
    await s.setupMemberKey(PASS)
    const put = sent<MemberKey>('/api/v1/keys/me', 'PUT')
    const rec = put.body
    expect(fromB64url(rec.public_key)).toHaveLength(32)
    expect(rec.kdf).toMatchObject({ alg: 'argon2id' })
    expect(put.headers).toMatchObject({ 'X-KlubHub-CSRF': '1' })
    expect(s.state).toBe('not_setup')

    const draft = await s.startSetup()
    expect(draft.groups).toHaveLength(8)
    expect(s.kitText('Nachtwerk')).toContain(draft.groups[0])
    const [a, b] = draft.challenge
    await expect(s.finishSetup(['AAAAAAA', 'BBBBBBB'])).rejects.toMatchObject({ error: 'kit_mismatch' })
    expect(api.db.version).toBeNull()
    await s.finishSetup([draft.groups[a]!.toLowerCase(), ` ${draft.groups[b]} `])
    expect(s.state).toBe('unlocked')
    expect(s.setup).toBeNull()

    const setup = sent<OrgSetupInput>('/api/v1/keys/org/setup').body
    expect(setup.version).toBe(1)
    expect(setup.wraps).toEqual([{ recipient_kind: 'member', recipient_id: ME, wrap: expect.any(String) }])
    expect(setup.recovery.fingerprint).toBe(fingerprint(fromB64url(setup.recovery.public_key)))
  })

  it('shows this user\'s key fingerprint, the one owners see in the recipients list', async () => {
    const s = useSealedStore()
    await s.fetchStatus()
    expect(s.myFingerprint).toBe('')
    await s.setupMemberKey(PASS)
    const pub = sent<MemberKey>('/api/v1/keys/me', 'PUT').body.public_key
    expect(s.myFingerprint).toMatch(/^[0-9A-F]{4}( [0-9A-F]{4}){3}$/)
    expect(s.myFingerprint).toBe(keyFingerprint(pub))
  })

  it('says local_required when the API wants a local account (single sign-on)', async () => {
    const real = api.handler.getMockImplementation()!
    api.handler.mockImplementation(async (url: string, opts?: Sent) => {
      if (url === '/api/v1/keys/me') throw Object.assign(new Error('x'), { statusCode: 403, data: { error: 'local_identity_required' } })
      return real(url, opts)
    })
    const s = useSealedStore()
    await s.fetchStatus()
    expect(s.error).toBeNull()
    expect(s.localRequired).toBe(true)
    expect(s.state).toBe('local_required')
  })

  it('locks, refuses a wrong passphrase and unlocks the stored wrap with the right one', async () => {
    const { s } = await ownerSetUp()
    s.lock()
    expect(s.state).toBe('locked')
    expect(s.lockedBy).toBe('manual')
    await expect(s.unlock('wrong passphrase here')).rejects.toMatchObject({ error: 'wrong_passphrase' })
    expect(s.state).toBe('locked')
    await s.unlock(PASS)
    expect(s.state).toBe('unlocked')
    expect(s.unlockedVersion).toBe(1)
  })

  it('auto-locks after 30 minutes idle and on sign-out', async () => {
    const { s } = await ownerSetUp()
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(Date.now() + IDLE_LOCK_MS - 1000)
    s.checkIdle()
    expect(s.unlocked).toBe(true)
    vi.setSystemTime(Date.now() + 2000)
    s.checkIdle()
    expect(s.unlocked).toBe(false)
    expect(s.lockedBy).toBe('idle')

    vi.useRealTimers()
    await s.unlock(PASS)
    useSessionStore().me = null
    await Promise.resolve()
    await vi.waitFor(() => expect(s.unlocked).toBe(false))
    expect(s.lockedBy).toBe('signout')
  })

  it('a member without a wrap is no_access until an owner grants it', async () => {
    await ownerSetUp()
    const s = useSealedStore()
    const who = await s.fetchRecipients()
    expect(who?.members.find(m => m.user_id === 'user-lena')?.has_wrap).toBe(false)
    await s.grant([{ kind: 'member', id: 'user-lena', public_key: b64url(api.db.lena.publicKey) }])
    const w = api.db.wraps.find(x => x.recipient_id === 'user-lena')!
    const osk = await unwrapOsk(api.db.lena.privateKey, w.wrap, oskAad(TENANT, 1, 'member', 'user-lena'))
    expect(osk).toHaveLength(32)
    expect(s.recipients?.members.find(m => m.user_id === 'user-lena')?.has_wrap).toBe(true)
  })

  it('provisions a door device the door can open offline', async () => {
    const { s } = await ownerSetUp()
    const ban = useBanListStore()
    await ban.add({ name: 'Viktor Brandt', reason: 'Fight at the bar' }, later(30))
    await s.provisionDevice('dev-1', b64url(api.db.door.publicKey))
    const w = api.db.wraps.find(x => x.recipient_kind === 'device')!
    expect(w).toMatchObject({ recipient_id: 'dev-1', version: 1 })
    const r = await openDoorBan({ key_version: 1, wrap: w.wrap, ban_entries: api.db.ban }, api.db.door.privateKey, TENANT, 'dev-1')
    expect(r.entries).toEqual([expect.objectContaining({ name: 'Viktor Brandt', reason: 'Fight at the bar' })])
    await expect(openDoorBan({ key_version: 1, wrap: w.wrap, ban_entries: [] }, api.db.door.privateKey, TENANT, 'dev-2')).rejects.toThrow()
  })

  it('rotates: a new version for everyone with access, every entry re-encrypted', async () => {
    const { s } = await ownerSetUp()
    const ban = useBanListStore()
    await ban.add({ name: 'Viktor Brandt', reason: 'Fight' }, later(30))
    await ban.add({ name: 'Mia Klein', reason: 'Theft', email: 'mia@example.org' }, later(60))
    await s.grant([{ kind: 'member', id: 'user-lena', public_key: b64url(api.db.lena.publicKey) }])
    await s.provisionDevice('dev-1', b64url(api.db.door.publicKey))
    const before = api.db.ban.map(e => e.entry_sealed)
    api.db.pending = true
    await s.fetchStatus()
    expect(s.org?.status).toBe('rotation_pending')

    await s.rotate()
    expect(api.db.version).toBe(2)
    expect(s.unlockedVersion).toBe(2)
    expect(s.org?.status).toBe('ready')
    const body = sent<RotateInput>('/api/v1/keys/org/rotate').body
    expect(body).toMatchObject({ from_version: 1, to_version: 2 })
    expect(body.wraps.map(w => `${w.recipient_kind}:${w.recipient_id}`).sort()).toEqual(['device:dev-1', 'member:user-lena', `member:${ME}`])
    expect(api.db.ban.map(e => e.entry_sealed)).not.toEqual(before)
    await ban.load()
    expect(ban.sorted.map(e => e.plain?.name)).toEqual(['Mia Klein', 'Viktor Brandt'])
    // Lena and the door open version 2; the old kit still works (same recovery key).
    const lena = api.db.wraps.find(x => x.version === 2 && x.recipient_id === 'user-lena')!
    await expect(unwrapOsk(api.db.lena.privateKey, lena.wrap, oskAad(TENANT, 2, 'member', 'user-lena'))).resolves.toHaveLength(32)
    expect(body.recovery.public_key).toBe(api.db.recovery!.public_key)
  })

  it('refuses to rotate to a recovery key that does not match the kit fingerprint', async () => {
    const { s } = await ownerSetUp()
    api.db.recovery = { ...api.db.recovery!, public_key: b64url(x25519Keypair().publicKey) }
    await expect(s.rotate()).rejects.toMatchObject({ error: 'recovery_mismatch' })
    expect(api.db.version).toBe(1)
  })

  it('refuses to rotate while an entry does not open', async () => {
    const { s } = await ownerSetUp()
    api.db.ban.push({ id: 'junk', key_version: 1, entry_sealed: b64url(randomBytes(40)), expires_at: later(3), created_at: '', updated_at: '' })
    await expect(s.rotate()).rejects.toMatchObject({ error: 'unreadable_entries', count: 1 })
    expect(api.db.version).toBe(1)
  })

  it('recovers with the kit: new member key, new wrap, the ban list still opens', async () => {
    const { groups } = await ownerSetUp()
    await useBanListStore().add({ name: 'Viktor Brandt', reason: 'Fight' }, later(30))
    const oldPub = api.db.keys.get(ME)!.public_key

    // A new browser session: nothing in memory, passphrase forgotten.
    setActivePinia(createPinia())
    signIn()
    const s = useSealedStore()
    await s.fetchStatus()
    expect(s.state).toBe('locked')
    await expect(s.recoverWithKit('abc', 'new passphrase long')).rejects.toMatchObject({ error: 'kit_length' })
    const typo = groups.join('')
    await expect(s.recoverWithKit(`${typo.slice(0, 5)}${typo[5] === 'A' ? 'B' : 'A'}${typo.slice(6)}`, 'new passphrase long')).rejects.toMatchObject({ error: 'kit_checksum' })
    await s.recoverWithKit(groups.join('-').toLowerCase(), 'new passphrase long')
    expect(s.state).toBe('unlocked')
    expect(api.db.keys.get(ME)!.public_key).not.toBe(oldPub)
    const ban = useBanListStore()
    await ban.load()
    expect(ban.sorted[0]?.plain?.name).toBe('Viktor Brandt')
    s.lock()
    await s.unlock('new passphrase long')
    expect(s.unlocked).toBe(true)
  })

  it('refuses a kit from another collective by its fingerprint', async () => {
    await ownerSetUp()
    setActivePinia(createPinia())
    signIn()
    const other = useSealedStore()
    await other.fetchStatus()
    const fresh = await other.startSetup()
    other.cancelSetup()
    await expect(other.recoverWithKit(fresh.groups.join(' '), 'new passphrase long')).rejects.toMatchObject({ error: 'kit_wrong_org' })
  })

  it('follows a rotation done elsewhere without asking for the passphrase again', async () => {
    const { s } = await ownerSetUp()
    await s.unlock(PASS) // member private key in memory
    const other = useSealedStore()
    expect(other).toBe(s)
    await s.rotate() // stands in for "another owner rotated": the server now has version 2 and a wrap for us
    s.unlockedVersion = 1 // pretend this tab still holds version 1
    await s.fetchStatus()
    expect(s.unlockedVersion).toBe(2)
  })
})

describe('useBanListStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    api = fakeApi()
    vi.stubGlobal('$fetch', api.handler)
    signIn()
  })
  afterEach(() => useSealedStore().lock())

  it('refuses writes while locked and clears on lock', async () => {
    const ban = useBanListStore()
    await expect(ban.add({ name: 'A', reason: 'B' }, later(3))).rejects.toMatchObject({ error: 'locked' })
    const { s } = await ownerSetUp()
    await ban.add({ name: 'A B', reason: 'B' }, later(3))
    expect(ban.entries).toHaveLength(1)
    s.lock()
    await Promise.resolve()
    expect(ban.entries).toHaveLength(0)
  })

  it('adds with a client uuid, edits and removes; the server only sees ciphertext', async () => {
    await ownerSetUp()
    const ban = useBanListStore()
    const e = await ban.add({ name: ' Viktor  Brandt ', reason: 'Fight', note: 'Ask Kim' }, later(30))
    expect(e.id).toMatch(/^[0-9a-f-]{36}$/)
    const post = sent<Record<string, unknown>>('/api/v1/ban-list', 'POST')
    expect(Object.keys(post.body).sort()).toEqual(['entry_sealed', 'expires_at', 'id', 'key_version'])
    expect(JSON.stringify(post.body)).not.toContain('Viktor')
    await ban.update(e.id, { name: 'Viktor Brandt', reason: 'Fight and theft' }, later(60))
    await ban.load()
    expect(ban.sorted[0]?.plain).toEqual({ name: 'Viktor Brandt', reason: 'Fight and theft' })
    await ban.remove(e.id)
    expect(ban.entries).toHaveLength(0)
    await ban.remove('gone') // already gone: fine
  })

  it('keeps unreadable entries visible as such', async () => {
    await ownerSetUp()
    api.db.ban.push({ id: 'junk', key_version: 1, entry_sealed: b64url(randomBytes(40)), expires_at: later(3), created_at: '', updated_at: '' })
    const ban = useBanListStore()
    await ban.load()
    expect(ban.unreadable).toBe(1)
    expect(ban.entries[0]?.plain).toBeNull()
  })

  it('surfaces key_version_stale and retries after following the new key', async () => {
    await ownerSetUp()
    const s = useSealedStore()
    await s.unlock(PASS)
    const ban = useBanListStore()
    api.db.staleNext = true
    await expect(ban.add({ name: 'A B', reason: 'C' }, later(3))).rejects.toMatchObject({ error: 'key_version_stale', status: 409 })
    expect(await ban.refreshStale()).toBe(true)
    await ban.add({ name: 'A B', reason: 'C' }, later(3))
    expect(api.db.ban).toHaveLength(1)
  })

  it('loads nothing while locked and keeps a load error', async () => {
    const ban = useBanListStore()
    await ban.load()
    expect(api.handler).not.toHaveBeenCalledWith('/api/v1/ban-list', expect.anything())
    await ownerSetUp()
    api.handler.mockRejectedValueOnce(Object.assign(new Error('x'), { statusCode: 503, data: { error: 'unavailable' } }))
    await ban.load()
    expect(ban.error).toMatchObject({ error: 'unavailable' })
  })
})
