import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import type { ApiError } from '~/types/event'
import type {
  BanPlain, MemberKey, OrgKeyInfo, OrgSetupInput, Recipients, RecoveryWrap, RotateInput, WrapInput, BanListResponse,
} from '~/types/sealed'
import { apiFetch, toApiError } from '~/utils/api'
import { b64url, fromB64url, wipe } from '~/utils/sealed/bytes'
import { decryptBan, encryptBan } from '~/utils/sealed/ban'
import type { Progress } from '~/utils/sealed/kdf'
import {
  createMemberKey, fingerprint, generateOsk, generateRecoveryKit, groupMatches, kitFileText, memberIdCandidates, oskAad, parseKit,
  PASSPHRASE_MIN, pickChallenge, recoveryKeypair, type RecoveryKit, unlockMemberKey, unwrapOsk, wrapOsk,
} from '~/utils/sealed/keys'
import { useSessionStore } from './session'

/** Auto-lock after this long without a pointer, key or scroll event. */
export const IDLE_LOCK_MS = 30 * 60_000
const IDLE_CHECK_MS = 30_000

export type SealedWork = 'create' | 'unlock' | 'setup' | 'grant' | 'rotate' | 'recover' | 'provision'

/**
 * What the ENCRYPTION & BAN LIST section shows:
 * - no_key: this user has no member key yet (create a passphrase)
 * - not_setup: the collective has no sealed key yet (owners: the wizard)
 * - no_access: set up, but nobody gave this user a wrap (ask an owner / owners: recover with kit)
 * - locked / unlocked: this user has a wrap; the keys are (not) in memory
 */
export type SealedState = 'loading' | 'error' | 'no_key' | 'not_setup' | 'no_access' | 'locked' | 'unlocked'

const fail = (error: string, extra: Record<string, unknown> = {}): ApiError => ({ error, ...extra }) as ApiError

/** The setup the wizard is walking through: nothing reaches the server until FINISH. */
export interface SetupDraft {
  groups: string[]
  fingerprint: string
  /** The two group positions (0-based) to re-type. */
  challenge: [number, number]
}

/**
 * Sealed tier (P2.6): the signed-in user's member key, the collective's
 * sealed key (OSK) status, and — while unlocked — the member private key
 * and the OSK in memory only. They are held in this closure (not in state,
 * never in storage, never serialised with the SSR payload), wiped on LOCK,
 * after 30 minutes idle and on sign-out.
 */
export const useSealedStore = defineStore('sealed', () => {
  const session = useSessionStore()

  const memberKey = ref<MemberKey | null>(null)
  const org = ref<OrgKeyInfo | null>(null)
  const loaded = ref(false)
  const loading = ref(false)
  const error = ref<ApiError | null>(null)
  /** The OSK version held in memory; null while locked. */
  const unlockedVersion = ref<number | null>(null)
  const working = ref<SealedWork | null>(null)
  /** Argon2id progress 0–1 while deriving (null when not deriving). */
  const progress = ref<number | null>(null)
  const lockedBy = ref<'idle' | 'manual' | 'signout' | null>(null)
  const recipients = ref<Recipients | null>(null)
  const recipientsError = ref<ApiError | null>(null)
  const setup = ref<SetupDraft | null>(null)

  let memberPriv: Uint8Array | null = null
  let osk: Uint8Array | null = null
  let pending: { kit: RecoveryKit, osk: Uint8Array } | null = null

  const tenant = computed(() => session.me?.org_id ?? '')
  const isOwner = computed(() => !!session.me?.roles.includes('owner'))
  const unlocked = computed(() => unlockedVersion.value !== null)
  const myId = computed(() => memberIdCandidates(session.me?.sub ?? '')[0] ?? '')

  const state = computed<SealedState>(() => {
    if (!loaded.value) return error.value ? 'error' : 'loading'
    if (unlocked.value) return 'unlocked'
    if (!memberKey.value) return 'no_key'
    if (!org.value || org.value.status === 'not_setup') return 'not_setup'
    if (!org.value.my_wrap) return 'no_access'
    return 'locked'
  })

  // ---------------------------------------------------------------- helpers

  async function call<T>(fn: () => Promise<T>): Promise<T> {
    try {
      return await fn()
    } catch (e) {
      throw toApiError(e)
    }
  }

  /** A write against a version that is no longer active: reload (following the new version when possible), then rethrow. */
  async function followOnConflict<T>(fn: () => Promise<T>): Promise<T> {
    try {
      return await call(fn)
    } catch (e) {
      if ((e as ApiError).error === 'version_conflict') await fetchStatus()
      throw e
    }
  }

  function onProgress(): Progress {
    progress.value = 0
    return (f: number) => {
      progress.value = f
    }
  }

  async function busy<T>(what: SealedWork, fn: () => Promise<T>): Promise<T> {
    working.value = what
    try {
      return await fn()
    } finally {
      working.value = null
      progress.value = null
    }
  }

  /** Open this user's wrap of the active OSK with the member private key in memory. */
  async function openMyWrap(priv: Uint8Array, info: OrgKeyInfo): Promise<Uint8Array> {
    if (!info.my_wrap || info.version === null) throw fail('no_access')
    for (const id of memberIdCandidates(session.me?.sub ?? '')) {
      try {
        return await unwrapOsk(priv, info.my_wrap, oskAad(tenant.value, info.version, 'member', id))
      } catch {
        // Try the next form of the user id.
      }
    }
    throw fail('wrap_unreadable')
  }

  function hold(priv: Uint8Array | null, key: Uint8Array, version: number) {
    if (priv && priv !== memberPriv) {
      wipe(memberPriv)
      memberPriv = priv
    }
    if (key !== osk) wipe(osk)
    osk = key
    unlockedVersion.value = version
    lockedBy.value = null
    startIdle()
  }

  // ---------------------------------------------------------------- status

  /** Load /keys/me and /keys/org. Never throws (the error is kept). */
  async function fetchStatus(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const [me, info] = await Promise.all([
        apiFetch<MemberKey>('/api/v1/keys/me').catch((e) => {
          const err = toApiError(e)
          if (err.status === 404 || err.error === 'no_member_key') return null
          throw e
        }),
        apiFetch<OrgKeyInfo>('/api/v1/keys/org'),
      ])
      memberKey.value = me
      org.value = info
      loaded.value = true
      await followVersion()
    } catch (e) {
      error.value = toApiError(e)
    } finally {
      loading.value = false
    }
  }

  /**
   * After a reload of the status: the OSK in memory still matches the
   * active version, or it is re-opened from the new wrap (someone rotated),
   * or the keys lock.
   */
  async function followVersion(): Promise<void> {
    const info = org.value
    if (unlockedVersion.value === null || !info) return
    if (info.status === 'not_setup' || info.version === null) return lock('manual')
    if (info.version === unlockedVersion.value) return
    if (memberPriv && info.my_wrap) {
      try {
        hold(null, await openMyWrap(memberPriv, info), info.version)
        return
      } catch {
        // Fall through: lock and let the user unlock again.
      }
    }
    lock('manual')
  }

  // ---------------------------------------------------------------- member key

  /** Create this user's member key (passphrase ≥ 12 characters). Throws an ApiError. */
  async function setupMemberKey(passphrase: string): Promise<void> {
    if (passphrase.length < PASSPHRASE_MIN) throw fail('passphrase_short')
    await busy('create', async () => {
      const kp = await createMemberKey(passphrase, { onProgress: onProgress() })
      try {
        await call(() => apiFetch('/api/v1/keys/me', { method: 'PUT', body: kp.record }))
      } finally {
        // Not kept: nothing can be opened with it yet (UNLOCK asks for the passphrase once access is given).
        wipe(kp.privateKey)
      }
      memberKey.value = kp.record
      await fetchStatus()
    })
  }

  /** Unlock: passphrase → member private key → this user's wrap → OSK, all in memory. Throws an ApiError. */
  async function unlock(passphrase: string): Promise<void> {
    await busy('unlock', async () => {
      if (!loaded.value) await fetchStatus()
      const rec = memberKey.value
      const info = org.value
      if (!rec) throw fail('no_member_key')
      if (!info || info.status === 'not_setup' || info.version === null) throw fail('not_setup')
      if (!info.my_wrap) throw fail('no_access')
      let priv: Uint8Array
      try {
        priv = await unlockMemberKey(passphrase, rec, { onProgress: onProgress() })
      } catch {
        // A malformed record would be a server bug; to the user it is the same: it did not unlock.
        throw fail('wrong_passphrase')
      }
      try {
        hold(priv, await openMyWrap(priv, info), info.version)
      } catch (e) {
        wipe(priv)
        throw e
      }
    })
  }

  /** Forget the keys in memory. */
  function lock(reason: 'idle' | 'manual' | 'signout' = 'manual'): void {
    wipe(memberPriv, osk)
    memberPriv = null
    osk = null
    cancelSetup()
    const was = unlockedVersion.value !== null
    unlockedVersion.value = null
    recipients.value = null
    stopIdle()
    if (was || reason !== 'manual') lockedBy.value = reason
  }

  // ---------------------------------------------------------------- org setup (owners)

  /** Start the wizard: a fresh OSK and recovery kit, kept here until FINISH. */
  async function startSetup(): Promise<SetupDraft> {
    cancelSetup()
    const kit = await generateRecoveryKit()
    pending = { kit, osk: generateOsk() }
    setup.value = { groups: kit.groups, fingerprint: kit.fingerprint, challenge: pickChallenge() }
    return setup.value
  }

  function cancelSetup() {
    if (pending) wipe(pending.kit.secret, pending.kit.privateKey, pending.osk)
    pending = null
    setup.value = null
  }

  /** The kit as a text file (for DOWNLOAD KIT). */
  function kitText(orgName: string): string {
    if (!setup.value) return ''
    return kitFileText({ groups: setup.value.groups, fingerprint: setup.value.fingerprint, org: orgName, createdAt: new Date().toISOString().slice(0, 10) })
  }

  /**
   * FINISH: the two re-typed groups must match; then the OSK (version 1)
   * is wrapped to this user and to the recovery key and sent. Throws an
   * ApiError (kit_mismatch, mfa_required, reauthentication_required, 409 …).
   */
  async function finishSetup(answers: [string, string]): Promise<void> {
    const draft = setup.value
    if (!draft || !pending) throw fail('not_setup')
    if (!answers.every((a, i) => groupMatches(a, draft.groups[draft.challenge[i]!]!))) throw fail('kit_mismatch')
    const rec = memberKey.value
    if (!rec) throw fail('no_member_key')
    await busy('setup', async () => {
      const p = pending!
      const version = 1
      const body: OrgSetupInput = {
        version,
        recovery: {
          public_key: b64url(p.kit.publicKey),
          fingerprint: p.kit.fingerprint,
          wrap: await wrapOsk(p.osk, p.kit.publicKey, oskAad(tenant.value, version, 'recovery', null)),
        },
        wraps: [{ recipient_kind: 'member', recipient_id: myId.value, wrap: await wrapOsk(p.osk, rec.public_key, oskAad(tenant.value, version, 'member', myId.value)) }],
      }
      await call(() => apiFetch('/api/v1/keys/org/setup', { method: 'POST', body }))
      const key = p.osk.slice()
      cancelSetup()
      hold(null, key, version)
      await fetchStatus()
    })
  }

  // ---------------------------------------------------------------- access (owners, unlocked)

  /** Members and door devices of the active version. Never throws (recipientsError). */
  async function fetchRecipients(): Promise<Recipients | null> {
    recipientsError.value = null
    try {
      recipients.value = await apiFetch<Recipients>('/api/v1/keys/org/recipients')
    } catch (e) {
      recipientsError.value = toApiError(e)
    }
    return recipients.value
  }

  function needOsk(): { key: Uint8Array, version: number } {
    if (!osk || unlockedVersion.value === null) throw fail('locked')
    return { key: osk, version: unlockedVersion.value }
  }

  /** Wrap the OSK to members / devices that have a public key. Throws an ApiError. */
  async function grant(targets: { kind: 'member' | 'device', id: string, public_key: string }[]): Promise<void> {
    if (!targets.length) return
    await busy('grant', async () => {
      const { key, version } = needOsk()
      const wraps: WrapInput[] = []
      for (const t of targets) {
        wraps.push({ recipient_kind: t.kind, recipient_id: t.id, wrap: await wrapOsk(key, t.public_key, oskAad(tenant.value, version, t.kind, t.id)) })
      }
      await followOnConflict(() => apiFetch('/api/v1/keys/org/wraps', { method: 'POST', body: { version, wraps } }))
      await fetchRecipients()
    })
  }

  /** PROVISION a door device (door.device.manage): the OSK sealed to its public key. Throws an ApiError. */
  async function provisionDevice(deviceId: string, publicKey: string): Promise<void> {
    await busy('provision', async () => {
      const { key, version } = needOsk()
      const wrap = await wrapOsk(key, publicKey, oskAad(tenant.value, version, 'device', deviceId))
      await followOnConflict(() => apiFetch(`/api/v1/keys/devices/${deviceId}/wrap`, { method: 'PUT', body: { version, wrap } }))
    })
    if (isOwner.value) await fetchRecipients()
  }

  /**
   * ROTATE NOW: a new OSK version wrapped to everyone who still has access
   * (members and live devices with a wrap, this user, the same recovery
   * key — the kit keeps working), and every ban entry re-encrypted under
   * it, in one request. Throws an ApiError.
   */
  async function rotate(): Promise<void> {
    await busy('rotate', async () => {
      const { key, version: from } = needOsk()
      const rec = memberKey.value
      if (!rec) throw fail('no_member_key')
      const [who, recovery, list] = await Promise.all([
        call(() => apiFetch<Recipients>('/api/v1/keys/org/recipients')),
        call(() => apiFetch<RecoveryWrap>('/api/v1/keys/org/recovery')),
        call(() => apiFetch<BanListResponse>('/api/v1/ban-list')),
      ])
      const plain: { id: string, p: BanPlain }[] = []
      let bad = 0
      for (const e of list.entries ?? []) {
        try {
          plain.push({ id: e.id, p: await decryptBan(key, tenant.value, e.id, e.key_version, e.entry_sealed) })
        } catch {
          bad++
        }
      }
      if (bad) throw fail('unreadable_entries', { count: bad })
      // The new key goes to the same recovery key the kit holds: refuse one whose fingerprint changed.
      const want = org.value?.recovery_fingerprint?.toLowerCase()
      const recoveryFp = recoveryFingerprint(recovery.public_key)
      if (!recoveryFp || recoveryFp !== recovery.fingerprint?.toLowerCase() || (want && recoveryFp !== want)) throw fail('recovery_mismatch')
      const to = from + 1
      const next = generateOsk()
      try {
        const wraps: WrapInput[] = [{ recipient_kind: 'member', recipient_id: myId.value, wrap: await wrapOsk(next, rec.public_key, oskAad(tenant.value, to, 'member', myId.value)) }]
        for (const m of who.members) {
          if (!m.has_wrap || !m.public_key || m.user_id === myId.value) continue
          wraps.push({ recipient_kind: 'member', recipient_id: m.user_id, wrap: await wrapOsk(next, m.public_key, oskAad(tenant.value, to, 'member', m.user_id)) })
        }
        for (const d of who.devices) {
          if (!d.has_wrap || !d.public_key || d.revoked) continue
          wraps.push({ recipient_kind: 'device', recipient_id: d.device_id, wrap: await wrapOsk(next, d.public_key, oskAad(tenant.value, to, 'device', d.device_id)) })
        }
        const body: RotateInput = {
          from_version: from,
          to_version: to,
          recovery: { public_key: recovery.public_key, fingerprint: recovery.fingerprint, wrap: await wrapOsk(next, recovery.public_key, oskAad(tenant.value, to, 'recovery', null)) },
          wraps,
          ban_entries: await Promise.all(plain.map(async x => ({ id: x.id, entry_sealed: await encryptBan(next, tenant.value, x.id, to, x.p) }))),
        }
        await call(() => apiFetch('/api/v1/keys/org/rotate', { method: 'POST', body }))
      } catch (e) {
        wipe(next)
        const err = e as ApiError
        if (err.error === 'version_conflict') await fetchStatus()
        throw e
      }
      hold(null, next, to)
      await fetchStatus()
      await fetchRecipients()
    })
  }

  // ---------------------------------------------------------------- recovery (owners)

  /**
   * RECOVER WITH KIT: the typed kit re-derives the recovery key, which opens
   * the OSK; a new member key under the new passphrase replaces this user's
   * key, and the OSK is wrapped to it. Throws an ApiError (kit_* codes,
   * mfa_required, reauthentication_required …).
   */
  async function recoverWithKit(kit: string, passphrase: string): Promise<void> {
    if (passphrase.length < PASSPHRASE_MIN) throw fail('passphrase_short')
    const parsed = parseKit(kit)
    if (parsed.error) throw fail(`kit_${parsed.error}`, { count: parsed.count })
    await busy('recover', async () => {
      const kp = await recoveryKeypair(parsed.secret!)
      wipe(parsed.secret)
      try {
        if (!loaded.value) await fetchStatus()
        const want = org.value?.recovery_fingerprint
        if (want && want.toLowerCase() !== fingerprint(kp.publicKey)) throw fail('kit_wrong_org')
        const r = await call(() => apiFetch<RecoveryWrap>('/api/v1/keys/org/recovery'))
        const version = r.version ?? org.value?.version ?? null
        if (version === null) throw fail('not_setup')
        if (r.fingerprint && r.fingerprint.toLowerCase() !== fingerprint(kp.publicKey)) throw fail('kit_wrong_org')
        let key: Uint8Array
        try {
          key = await unwrapOsk(kp.privateKey, r.wrap, oskAad(tenant.value, version, 'recovery', null))
        } catch {
          throw fail('kit_unreadable')
        }
        const mk = await createMemberKey(passphrase, { onProgress: onProgress() })
        try {
          await call(() => apiFetch('/api/v1/keys/me', { method: 'PUT', body: mk.record }))
          memberKey.value = mk.record
          const wrap = await wrapOsk(key, mk.publicKey, oskAad(tenant.value, version, 'member', myId.value))
          await call(() => apiFetch('/api/v1/keys/org/wraps', { method: 'POST', body: { version, wraps: [{ recipient_kind: 'member', recipient_id: myId.value, wrap }] } }))
        } catch (e) {
          wipe(mk.privateKey, key)
          throw e
        }
        hold(mk.privateKey, key, version)
        await fetchStatus()
      } finally {
        wipe(kp.privateKey)
      }
    })
  }

  // ---------------------------------------------------------------- ban entries (for the ban list store)

  /** Encrypt one entry under the OSK in memory. Throws ApiError locked. */
  async function sealEntry(id: string, p: BanPlain): Promise<{ entry_sealed: string, key_version: number }> {
    const { key, version } = needOsk()
    return { entry_sealed: await encryptBan(key, tenant.value, id, version, p), key_version: version }
  }

  /** Decrypt one entry; null when it does not open (other version, tampered). */
  async function openEntry(id: string, version: number, sealed: string): Promise<BanPlain | null> {
    if (!osk) return null
    try {
      return await decryptBan(osk, tenant.value, id, version, sealed)
    } catch {
      return null
    }
  }

  /** After key_version_stale: reload the status and follow the new version. True when still unlocked. */
  async function refreshKeys(): Promise<boolean> {
    await fetchStatus()
    return unlocked.value
  }

  // ---------------------------------------------------------------- auto-lock

  let lastActivity = Date.now()
  let idleTimer: ReturnType<typeof setInterval> | null = null
  const EVENTS = ['pointerdown', 'keydown', 'scroll', 'touchstart'] as const
  const bump = () => {
    lastActivity = Date.now()
  }
  const checkIdle = () => {
    if (unlockedVersion.value !== null && Date.now() - lastActivity >= IDLE_LOCK_MS) lock('idle')
  }
  const onVisible = () => {
    if (document.visibilityState === 'visible') checkIdle()
  }

  function startIdle() {
    lastActivity = Date.now()
    if (idleTimer || typeof window === 'undefined') return
    idleTimer = setInterval(checkIdle, IDLE_CHECK_MS)
    for (const e of EVENTS) window.addEventListener(e, bump, { passive: true, capture: true })
    document.addEventListener('visibilitychange', onVisible)
  }

  function stopIdle() {
    if (idleTimer) clearInterval(idleTimer)
    idleTimer = null
    if (typeof window === 'undefined') return
    for (const e of EVENTS) window.removeEventListener(e, bump, { capture: true })
    document.removeEventListener('visibilitychange', onVisible)
  }

  // Signing out (or another user signing in) locks.
  watch(() => session.me?.sub ?? null, (sub, was) => {
    if (was && sub !== was) {
      lock('signout')
      memberKey.value = null
      org.value = null
      loaded.value = false
    }
  })

  return {
    memberKey, org, loaded, loading, error, unlockedVersion, working, progress, lockedBy, recipients, recipientsError, setup,
    tenant, isOwner, unlocked, state, myId,
    fetchStatus, setupMemberKey, unlock, lock, startSetup, cancelSetup, kitText, finishSetup, fetchRecipients, grant, provisionDevice,
    rotate, recoverWithKit, sealEntry, openEntry, refreshKeys, checkIdle,
  }
})

/** The fingerprint of a base64url public key, or '' when it is not one. */
function recoveryFingerprint(pub: string): string {
  try {
    return fingerprint(fromB64url(pub))
  } catch {
    return ''
  }
}
