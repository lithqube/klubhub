/**
 * Byte helpers for the sealed tier (P2.6): base64url (the JSON encoding of
 * every key, wrap and sealed blob), concatenation, UTF-8, randomness and
 * a constant-time compare. Pure, no Vue.
 */
export { constantTimeEqual } from '~/utils/doorPin'

const enc = new TextEncoder()
const dec = new TextDecoder()

export const utf8 = (s: string): Uint8Array => new Uint8Array(enc.encode(s))
export const fromUtf8 = (b: Uint8Array): string => dec.decode(b)

/** base64url without padding (RFC 4648 §5), as Go's base64.RawURLEncoding. */
export function b64url(bytes: Uint8Array): string {
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

/**
 * Decode base64url; tolerant of padding and of the standard alphabet.
 * Throws on anything else.
 */
export function fromB64url(s: string): Uint8Array {
  if (typeof s !== 'string' || !/^[A-Za-z0-9\-_+/]*={0,2}$/.test(s)) throw new Error('invalid base64url')
  const std = s.replace(/-/g, '+').replace(/_/g, '/').replace(/=+$/, '')
  if (std.length % 4 === 1) throw new Error('invalid base64url')
  const bin = atob(std + '='.repeat((4 - (std.length % 4)) % 4))
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

export function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0))
  let o = 0
  for (const p of parts) {
    out.set(p, o)
    o += p.length
  }
  return out
}

export function randomBytes(n: number): Uint8Array {
  return globalThis.crypto.getRandomValues(new Uint8Array(n))
}

export function toHex(bytes: Uint8Array): string {
  return [...bytes].map(b => b.toString(16).padStart(2, '0')).join('')
}

/** Overwrite secret bytes once they are no longer needed (best effort in JS). */
export function wipe(...arrays: (Uint8Array | null | undefined)[]): void {
  for (const a of arrays) a?.fill(0)
}

/** A copy backed by its own ArrayBuffer (WebCrypto wants a plain BufferSource). */
export const buf = (b: Uint8Array): BufferSource => new Uint8Array(b) as BufferSource
