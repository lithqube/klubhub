/**
 * Sealed-tier primitives (P2.6 contract, "Cryptography"). Everything is
 * encrypted in the browser; the server only ever sees the output.
 *
 * - seal(pub, plaintext, aad): ephemeral X25519 keypair;
 *   k = HKDF-SHA256(ikm = X25519(eph, pub), salt = eph_pub ‖ pub,
 *   info = "klubhub-seal-v1"); AES-256-GCM with a random 96-bit nonce and
 *   AAD = the context string. Output: 0x01 ‖ eph_pub(32) ‖ nonce(12) ‖ ct+tag.
 * - aesSeal(key, plaintext, aad): AES-256-GCM under a 32-byte key.
 *   Output: 0x01 ‖ nonce(12) ‖ ct+tag (ban entries, member private keys).
 *
 * "v1" in the contract is the single version byte 0x01.
 * X25519 comes from @noble/curves; HKDF and AES-GCM from WebCrypto.
 */
import { x25519 } from '@noble/curves/ed25519.js'
import { buf, concat, randomBytes, utf8, wipe } from './bytes'

export const SEAL_VERSION = 1
export const SEAL_INFO = 'klubhub-seal-v1'
export const KEY_BYTES = 32
export const NONCE_BYTES = 12
export const TAG_BYTES = 16
/** 0x01 ‖ eph_pub ‖ nonce ‖ tag (empty plaintext). */
export const SEAL_MIN = 1 + KEY_BYTES + NONCE_BYTES + TAG_BYTES
/** 0x01 ‖ nonce ‖ tag (empty plaintext). */
export const AES_MIN = 1 + NONCE_BYTES + TAG_BYTES

export type SealedErrorCode = 'malformed' | 'unreadable'

/** A blob that is not ours (malformed) or does not open with this key and context (unreadable). */
export class SealedError extends Error {
  constructor(readonly code: SealedErrorCode) {
    super(code === 'malformed' ? 'sealed blob is malformed' : 'sealed blob cannot be opened with this key')
    this.name = 'SealedError'
  }
}

const subtle = () => globalThis.crypto.subtle

/** A fresh X25519 keypair. */
export function x25519Keypair(): { privateKey: Uint8Array, publicKey: Uint8Array } {
  const privateKey = x25519.utils.randomSecretKey()
  return { privateKey, publicKey: x25519.getPublicKey(privateKey) }
}

export const x25519Public = (privateKey: Uint8Array): Uint8Array => x25519.getPublicKey(privateKey)

/** HKDF-SHA256 → 32 bytes (WebCrypto). */
export async function hkdfBytes(ikm: Uint8Array, salt: Uint8Array, info: string, length = KEY_BYTES): Promise<Uint8Array> {
  const base = await subtle().importKey('raw', buf(ikm), 'HKDF', false, ['deriveBits'])
  const bits = await subtle().deriveBits({ name: 'HKDF', hash: 'SHA-256', salt: buf(salt), info: buf(utf8(info)) }, base, length * 8)
  return new Uint8Array(bits)
}

async function aesKey(raw: Uint8Array): Promise<CryptoKey> {
  if (raw.length !== KEY_BYTES) throw new SealedError('malformed')
  return subtle().importKey('raw', buf(raw), { name: 'AES-GCM' }, false, ['encrypt', 'decrypt'])
}

async function gcmEncrypt(key: CryptoKey, nonce: Uint8Array, plaintext: Uint8Array, aad: string): Promise<Uint8Array> {
  return new Uint8Array(await subtle().encrypt({ name: 'AES-GCM', iv: buf(nonce), additionalData: buf(utf8(aad)) }, key, buf(plaintext)))
}

async function gcmDecrypt(key: CryptoKey, nonce: Uint8Array, ct: Uint8Array, aad: string): Promise<Uint8Array> {
  try {
    return new Uint8Array(await subtle().decrypt({ name: 'AES-GCM', iv: buf(nonce), additionalData: buf(utf8(aad)) }, key, buf(ct)))
  } catch {
    throw new SealedError('unreadable')
  }
}

async function sealKey(shared: Uint8Array, ephPub: Uint8Array, pub: Uint8Array): Promise<CryptoKey> {
  const k = await hkdfBytes(shared, concat(ephPub, pub), SEAL_INFO)
  try {
    return await aesKey(k)
  } finally {
    wipe(k)
  }
}

/** Seal plaintext to an X25519 public key (see the header for the format). */
export async function seal(pub: Uint8Array, plaintext: Uint8Array, aad: string): Promise<Uint8Array> {
  if (pub.length !== KEY_BYTES) throw new SealedError('malformed')
  const eph = x25519Keypair()
  let shared: Uint8Array
  try {
    shared = x25519.getSharedSecret(eph.privateKey, pub)
  } catch {
    wipe(eph.privateKey)
    throw new SealedError('malformed')
  }
  try {
    const key = await sealKey(shared, eph.publicKey, pub)
    const nonce = randomBytes(NONCE_BYTES)
    return concat(Uint8Array.of(SEAL_VERSION), eph.publicKey, nonce, await gcmEncrypt(key, nonce, plaintext, aad))
  } finally {
    wipe(eph.privateKey, shared)
  }
}

/** Open a sealed blob with the recipient's private key; throws SealedError. */
export async function openSealed(privateKey: Uint8Array, blob: Uint8Array, aad: string): Promise<Uint8Array> {
  if (blob.length < SEAL_MIN || blob[0] !== SEAL_VERSION || privateKey.length !== KEY_BYTES) throw new SealedError('malformed')
  const ephPub = blob.slice(1, 1 + KEY_BYTES)
  const nonce = blob.slice(1 + KEY_BYTES, 1 + KEY_BYTES + NONCE_BYTES)
  const ct = blob.slice(1 + KEY_BYTES + NONCE_BYTES)
  let shared: Uint8Array
  try {
    shared = x25519.getSharedSecret(privateKey, ephPub)
  } catch {
    // Low-order point: never produced by seal().
    throw new SealedError('unreadable')
  }
  try {
    const key = await sealKey(shared, ephPub, x25519.getPublicKey(privateKey))
    return await gcmDecrypt(key, nonce, ct, aad)
  } finally {
    wipe(shared)
  }
}

/** AES-256-GCM under a 32-byte key: 0x01 ‖ nonce ‖ ct+tag. */
export async function aesSeal(keyBytes: Uint8Array, plaintext: Uint8Array, aad: string): Promise<Uint8Array> {
  const key = await aesKey(keyBytes)
  const nonce = randomBytes(NONCE_BYTES)
  return concat(Uint8Array.of(SEAL_VERSION), nonce, await gcmEncrypt(key, nonce, plaintext, aad))
}

/** Open an aesSeal blob; throws SealedError. */
export async function aesOpen(keyBytes: Uint8Array, blob: Uint8Array, aad: string): Promise<Uint8Array> {
  if (blob.length < AES_MIN || blob[0] !== SEAL_VERSION) throw new SealedError('malformed')
  const key = await aesKey(keyBytes)
  return gcmDecrypt(key, blob.slice(1, 1 + NONCE_BYTES), blob.slice(1 + NONCE_BYTES), aad)
}
