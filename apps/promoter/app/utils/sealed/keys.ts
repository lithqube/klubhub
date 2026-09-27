/**
 * Sealed-tier keys (P2.6): member keys, the org sealed key (OSK), its wraps
 * and the recovery kit. Pure functions over bytes; the store decides what
 * stays in memory.
 *
 * - Member key: X25519 keypair; the private key is AES-256-GCM encrypted
 *   under Argon2id(passphrase) with AAD "klubhub-member-key-v1|<public key
 *   base64url>" (binds the ciphertext to its public key).
 * - OSK: 32 random bytes with a version, sealed to each recipient with AAD
 *   "klubhub-osk|<tenant>|<version>|<kind>|<recipient id>" (empty id for
 *   the recovery key).
 * - Recovery kit: 32 random bytes + a 3-byte checksum (first bytes of
 *   SHA-256 of the secret) = 35 bytes = 56 base32 characters, shown as 8
 *   groups of 7. Recovery keypair: X25519 private =
 *   HKDF-SHA256(secret, salt = empty, info = "klubhub-recovery-v1").
 *   Fingerprint: first 8 bytes of SHA-256(public key), hex.
 */
import { sha256 } from '@noble/hashes/sha2.js'
import type { KdfParams, MemberKey, RecipientKind } from '~/types/sealed'
import { b64url, constantTimeEqual, fromB64url, randomBytes, toHex, wipe } from './bytes'
import { deriveKey, newKdfParams, type Progress } from './kdf'
import { aesOpen, aesSeal, hkdfBytes, KEY_BYTES, openSealed, seal, SealedError, x25519Keypair, x25519Public } from './seal'

export const PASSPHRASE_MIN = 12
export const OSK_BYTES = 32
export const RECOVERY_INFO = 'klubhub-recovery-v1'

export type Derive = (passphrase: string, k: KdfParams, onProgress?: Progress) => Promise<Uint8Array>

// ---------------------------------------------------------------- context strings (AAD)

export const memberKeyAad = (publicKey: string) => `klubhub-member-key-v1|${publicKey}`

export const oskAad = (tenant: string, version: number, kind: RecipientKind, recipientId: string | null) =>
  `klubhub-osk|${tenant}|${version}|${kind}|${recipientId ?? ''}`

export const banAad = (tenant: string, entryId: string, version: number) => `klubhub-ban|${tenant}|${entryId}|${version}`

/**
 * The member recipient id for the signed-in user: the user uuid, which is
 * the session subject without its "local:" prefix. Both forms are tried
 * when opening one's own wrap.
 */
export function memberIdCandidates(sub: string): string[] {
  const bare = sub.replace(/^local:/, '')
  return bare === sub ? [sub] : [bare, sub]
}

// ---------------------------------------------------------------- member key

export interface MemberKeyPair {
  record: MemberKey
  privateKey: Uint8Array
  publicKey: Uint8Array
}

/** A new member keypair with the private key encrypted under the passphrase. */
export async function createMemberKey(passphrase: string, opts: { kdf?: KdfParams, derive?: Derive, onProgress?: Progress } = {}): Promise<MemberKeyPair> {
  const kdf = opts.kdf ?? newKdfParams()
  const { privateKey, publicKey } = x25519Keypair()
  const k = await (opts.derive ?? deriveKey)(passphrase, kdf, opts.onProgress)
  try {
    const pub = b64url(publicKey)
    const sealed = await aesSeal(k, privateKey, memberKeyAad(pub))
    return { record: { public_key: pub, private_sealed: b64url(sealed), kdf }, privateKey, publicKey }
  } finally {
    wipe(k)
  }
}

/** The private key of a member key record; throws SealedError('unreadable') for a wrong passphrase. */
export async function unlockMemberKey(passphrase: string, record: MemberKey, opts: { derive?: Derive, onProgress?: Progress } = {}): Promise<Uint8Array> {
  let blob: Uint8Array, pub: Uint8Array
  try {
    blob = fromB64url(record.private_sealed)
    pub = fromB64url(record.public_key)
  } catch {
    throw new SealedError('malformed')
  }
  const k = await (opts.derive ?? deriveKey)(passphrase, record.kdf, opts.onProgress)
  let priv: Uint8Array
  try {
    priv = await aesOpen(k, blob, memberKeyAad(record.public_key))
  } finally {
    wipe(k)
  }
  if (priv.length !== KEY_BYTES || !constantTimeEqual(x25519Public(priv), pub)) {
    wipe(priv)
    throw new SealedError('unreadable')
  }
  return priv
}

// ---------------------------------------------------------------- org sealed key

export const generateOsk = (): Uint8Array => randomBytes(OSK_BYTES)

/** Seal the OSK to a recipient's public key (base64url in, base64url out). */
export async function wrapOsk(osk: Uint8Array, publicKey: string | Uint8Array, aad: string): Promise<string> {
  const pub = typeof publicKey === 'string' ? fromB64url(publicKey) : publicKey
  return b64url(await seal(pub, osk, aad))
}

/** Open a wrap with the recipient's private key; throws SealedError. */
export async function unwrapOsk(privateKey: Uint8Array, wrap: string, aad: string): Promise<Uint8Array> {
  let blob: Uint8Array
  try {
    blob = fromB64url(wrap)
  } catch {
    throw new SealedError('malformed')
  }
  const osk = await openSealed(privateKey, blob, aad)
  if (osk.length !== OSK_BYTES) {
    wipe(osk)
    throw new SealedError('malformed')
  }
  return osk
}

// ---------------------------------------------------------------- recovery kit

const B32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
export const KIT_SECRET_BYTES = 32
const KIT_CHECK_BYTES = 3
export const KIT_GROUPS = 8
export const KIT_GROUP_LEN = 7
const KIT_CHARS = KIT_GROUPS * KIT_GROUP_LEN

export function base32(bytes: Uint8Array): string {
  let out = ''
  let bits = 0
  let value = 0
  for (const b of bytes) {
    value = (value << 8) | b
    bits += 8
    while (bits >= 5) {
      out += B32[(value >>> (bits - 5)) & 31]
      bits -= 5
    }
  }
  if (bits > 0) out += B32[(value << (5 - bits)) & 31]
  return out
}

export function fromBase32(s: string): Uint8Array {
  const out: number[] = []
  let bits = 0
  let value = 0
  for (const c of s) {
    const v = B32.indexOf(c)
    if (v < 0) throw new Error('invalid base32')
    value = (value << 5) | v
    bits += 5
    if (bits >= 8) {
      out.push((value >>> (bits - 8)) & 255)
      bits -= 8
    }
  }
  return Uint8Array.from(out)
}

const checksum = (secret: Uint8Array) => sha256(secret).slice(0, KIT_CHECK_BYTES)

/** The 8 groups of 7 base32 characters for a 32-byte secret (with its checksum). */
export function kitGroups(secret: Uint8Array): string[] {
  if (secret.length !== KIT_SECRET_BYTES) throw new Error('kit secret must be 32 bytes')
  const whole = new Uint8Array(KIT_SECRET_BYTES + KIT_CHECK_BYTES)
  whole.set(secret)
  whole.set(checksum(secret), KIT_SECRET_BYTES)
  const s = base32(whole)
  return Array.from({ length: KIT_GROUPS }, (_, i) => s.slice(i * KIT_GROUP_LEN, (i + 1) * KIT_GROUP_LEN))
}

/**
 * Typed kit text → the canonical characters: upper case, no spaces, dashes
 * or dots; 0/1/8 read as O/I/B (they are not base32 digits).
 */
export function normaliseKit(input: string): string {
  return input.toUpperCase().replace(/[\s\-_.·,]+/g, '').replace(/0/g, 'O').replace(/1/g, 'I').replace(/8/g, 'B')
}

export type KitParse = { secret: Uint8Array, error: null } | { secret: null, error: 'length' | 'chars' | 'checksum', count: number }

/** Parse a typed or pasted kit (spaces, case and dashes do not matter). */
export function parseKit(input: string): KitParse {
  const s = normaliseKit(input)
  if (/[^A-Z2-7]/.test(s)) return { secret: null, error: 'chars', count: s.length }
  if (s.length !== KIT_CHARS) return { secret: null, error: 'length', count: s.length }
  const whole = fromBase32(s)
  const secret = whole.slice(0, KIT_SECRET_BYTES)
  const check = whole.slice(KIT_SECRET_BYTES, KIT_SECRET_BYTES + KIT_CHECK_BYTES)
  if (!constantTimeEqual(check, checksum(secret))) {
    wipe(secret)
    return { secret: null, error: 'checksum', count: s.length }
  }
  return { secret, error: null }
}

/** One re-typed group matches the kit's group (same tolerance as parseKit). */
export const groupMatches = (typed: string, group: string) => normaliseKit(typed) === group

/** The recovery keypair of a kit secret (deterministic). */
export async function recoveryKeypair(secret: Uint8Array): Promise<{ privateKey: Uint8Array, publicKey: Uint8Array }> {
  const privateKey = await hkdfBytes(secret, new Uint8Array(0), RECOVERY_INFO)
  return { privateKey, publicKey: x25519Public(privateKey) }
}

/** First 8 bytes of SHA-256(public key), lower-case hex (16 characters). */
export function fingerprint(publicKey: Uint8Array): string {
  return toHex(sha256(publicKey).slice(0, 8))
}

/** "3F9A 01C2 77B0 E4D1" for reading aloud or comparing. */
export function formatFingerprint(fp: string | null | undefined): string {
  return (fp ?? '').toUpperCase().replace(/(.{4})(?=.)/g, '$1 ')
}

export interface RecoveryKit {
  secret: Uint8Array
  groups: string[]
  publicKey: Uint8Array
  privateKey: Uint8Array
  fingerprint: string
}

export async function generateRecoveryKit(): Promise<RecoveryKit> {
  const secret = randomBytes(KIT_SECRET_BYTES)
  const kp = await recoveryKeypair(secret)
  return { secret, groups: kitGroups(secret), ...kp, fingerprint: fingerprint(kp.publicKey) }
}

/** Two different group positions (0-based, ascending) for the re-type check. */
export function pickChallenge(rand: () => number = Math.random): [number, number] {
  const a = Math.floor(rand() * KIT_GROUPS) % KIT_GROUPS
  let b = Math.floor(rand() * (KIT_GROUPS - 1)) % (KIT_GROUPS - 1)
  if (b >= a) b++
  return a < b ? [a, b] : [b, a]
}

/** The downloadable kit (plain text; no secret other than the groups). */
export function kitFileText(k: { groups: string[], fingerprint: string, org: string, createdAt: string }): string {
  return [
    'KLUBHUB PROMOTER · SEALED DATA RECOVERY KIT',
    '',
    `Collective:  ${k.org}`,
    `Created:     ${k.createdAt}`,
    `Fingerprint: ${formatFingerprint(k.fingerprint)}`,
    '',
    'Recovery code (8 groups):',
    '',
    `  ${k.groups.slice(0, 4).join(' ')}`,
    `  ${k.groups.slice(4).join(' ')}`,
    '',
    'This code opens your collective\'s sealed data (the ban list) if every',
    'owner forgets their passphrase. Anyone with it can read that data.',
    'Keep it offline: print it or store it in a password manager, and delete',
    'this file from your downloads folder. KlubHub cannot recover it for you.',
    '',
    'To use it: Settings → ENCRYPTION & BAN LIST → RECOVER WITH KIT.',
    '',
  ].join('\n')
}

// ---------------------------------------------------------------- passphrase

export type Strength = 'short' | 'weak' | 'ok' | 'strong'

/**
 * A plain strength hint (not a policy beyond the 12-character minimum):
 * length, character variety and several words count.
 */
export function passphraseStrength(p: string): Strength {
  const s = normalisePassphraseForHint(p)
  if (s.length < PASSPHRASE_MIN) return 'short'
  const classes = [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9\s]/].filter(r => r.test(s)).length
  const words = s.trim().split(/\s+/).filter(w => w.length >= 3).length
  const distinct = new Set(s.toLowerCase()).size
  if (distinct < 6) return 'weak'
  if (s.length >= 20 || (s.length >= 16 && (classes >= 3 || words >= 4))) return 'strong'
  if (classes >= 2 || words >= 3) return 'ok'
  return 'weak'
}
const normalisePassphraseForHint = (p: string) => p.normalize('NFKC')

export const STRENGTH_TEXT: Record<Strength, string> = {
  short: `At least ${PASSPHRASE_MIN} characters.`,
  weak: 'Weak: add words or mix in numbers and symbols.',
  ok: 'OK. Four or more random words make it stronger.',
  strong: 'Strong.',
}
