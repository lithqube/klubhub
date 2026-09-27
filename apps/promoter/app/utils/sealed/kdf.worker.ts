/**
 * Argon2id off the main thread (see kdf.ts). One request per worker: the
 * page creates it, posts {id, passphrase, salt, m, t, p}, gets progress
 * messages and then {id, key} or {id, error}, and terminates it.
 */
import { argon2idAsync } from '@noble/hashes/argon2.js'

interface Req { id: number, passphrase: string, salt: string, m: number, t: number, p: number }

const ctx = self as unknown as {
  onmessage: ((e: MessageEvent<Req>) => void) | null
  postMessage: (msg: unknown, transfer?: Transferable[]) => void
}

function fromB64url(s: string): Uint8Array {
  const std = s.replace(/-/g, '+').replace(/_/g, '/').replace(/=+$/, '')
  const bin = atob(std + '='.repeat((4 - (std.length % 4)) % 4))
  return Uint8Array.from(bin, c => c.charCodeAt(0))
}

ctx.onmessage = async (e) => {
  const { id, passphrase, salt, m, t, p } = e.data
  let last = -1
  try {
    const key = await argon2idAsync(new TextEncoder().encode(passphrase), fromB64url(salt), {
      m, t, p, dkLen: 32, maxmem: 2 ** 31,
      onProgress: (f) => {
        const pct = Math.floor(f * 100)
        if (pct !== last) {
          last = pct
          ctx.postMessage({ id, progress: f })
        }
      },
    })
    ctx.postMessage({ id, key }, [key.buffer as ArrayBuffer])
  } catch (err) {
    ctx.postMessage({ id, error: err instanceof Error ? err.message : 'kdf failed' })
  }
}
