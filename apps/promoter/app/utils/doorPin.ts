/**
 * Offline manager-PIN check (P2.3): the bundle carries
 * PBKDF2-SHA256(pin, salt, iterations) → 32 bytes, base64. The device
 * derives the same with WebCrypto and compares in constant time. The server
 * re-verifies every add against the Argon2id hash, with lockout, so this
 * only saves a round trip — it is not the security boundary.
 */
import type { ManagerPinVerifier } from '~/types/door'

export function b64decode(s: string): Uint8Array {
  const bin = atob(s)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

export function b64encode(bytes: Uint8Array): string {
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin)
}

/** Equal length and bytes, without an early exit on the first difference. */
export function constantTimeEqual(a: Uint8Array, b: Uint8Array): boolean {
  let diff = a.length ^ b.length
  const n = Math.max(a.length, b.length)
  for (let i = 0; i < n; i++) diff |= (a[i] ?? 0) ^ (b[i] ?? 0)
  return diff === 0
}

export async function pbkdf2Sha256(pin: string, salt: Uint8Array, iterations: number, bytes = 32): Promise<Uint8Array> {
  const subtle = globalThis.crypto.subtle
  const key = await subtle.importKey('raw', new TextEncoder().encode(pin) as BufferSource, 'PBKDF2', false, ['deriveBits'])
  const bits = await subtle.deriveBits({ name: 'PBKDF2', hash: 'SHA-256', salt: salt as BufferSource, iterations }, key, bytes * 8)
  return new Uint8Array(bits)
}

export const PIN_RE = /^\d{6}$/

/** True when pin matches the manager PIN verifier. Malformed input is simply false. */
export async function verifyManagerPin(pin: string, v: ManagerPinVerifier | null): Promise<boolean> {
  if (!v || !PIN_RE.test(pin) || !(v.iterations > 0)) return false
  let salt: Uint8Array, want: Uint8Array
  try {
    salt = b64decode(v.salt)
    want = b64decode(v.hash)
  } catch {
    return false
  }
  const got = await pbkdf2Sha256(pin, salt, v.iterations, want.length || 32)
  return constantTimeEqual(got, want)
}
