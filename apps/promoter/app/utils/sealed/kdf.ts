/**
 * Passphrase → key for the member key (P2.6): Argon2id(passphrase, salt 16 B,
 * m = 64 MiB, t = 3, p = 1) → 32 bytes, params stored with the key.
 *
 * Argon2id in JavaScript takes about a second on a laptop and several on
 * a phone, so in the browser it runs in a module Web Worker (kdf.worker.ts)
 * that reports progress; where there is no Worker (unit tests, a worker
 * refused by the browser) it runs in-thread with the async variant, which
 * yields to the event loop. There is no switch for weaker parameters: tests
 * pass their own tiny params to deriveKey().
 */
import { argon2idAsync } from '@noble/hashes/argon2.js'
import type { KdfParams } from '~/types/sealed'
import { b64url, fromB64url, randomBytes, utf8 } from './bytes'

export const KDF_ALG = 'argon2id'
/** Production parameters (contract): m in KiB. */
export const KDF_DEFAULTS = { m: 65_536, t: 3, p: 1 } as const
export const SALT_BYTES = 16
/** Refuse parameters a tampered record could use to freeze the browser. */
const MAX = { m: 262_144, t: 10, p: 4 } as const

export type Progress = (fraction: number) => void

/** Fresh parameters for a new member key (production cost, random salt). */
export function newKdfParams(): KdfParams {
  return { alg: KDF_ALG, ...KDF_DEFAULTS, salt: b64url(randomBytes(SALT_BYTES)) }
}

/** The stored parameters are usable (known algorithm, sane bounds, 16-byte salt). */
export function validKdf(k: unknown): k is KdfParams {
  const p = k as Partial<KdfParams> | null
  if (!p || p.alg !== KDF_ALG) return false
  const int = (n: unknown, lo: number, hi: number) => typeof n === 'number' && Number.isInteger(n) && n >= lo && n <= hi
  if (!int(p.m, 8, MAX.m) || !int(p.t, 1, MAX.t) || !int(p.p, 1, MAX.p) || p.m! < 8 * p.p!) return false
  try {
    return fromB64url(p.salt ?? '').length === SALT_BYTES
  } catch {
    return false
  }
}

/** Passphrases are compared after NFKC so the same words typed on another keyboard still unlock. */
export const normalisePassphrase = (s: string) => s.normalize('NFKC')

/** Argon2id in this thread (tests, fallback). */
export async function argon2InThread(passphrase: string, k: KdfParams, onProgress?: Progress): Promise<Uint8Array> {
  return argon2idAsync(utf8(normalisePassphrase(passphrase)), fromB64url(k.salt), { m: k.m, t: k.t, p: k.p, dkLen: 32, onProgress, maxmem: 2 ** 31 })
}

interface WorkerReply { id: number, progress?: number, key?: Uint8Array, error?: string }

let seq = 0

function inWorker(passphrase: string, k: KdfParams, onProgress?: Progress): Promise<Uint8Array> {
  return new Promise((resolve, reject) => {
    // Literal form so Vite bundles the worker as its own chunk.
    const w = new Worker(new URL('./kdf.worker.ts', import.meta.url), { type: 'module' })
    const id = ++seq
    w.onmessage = (e: MessageEvent<WorkerReply>) => {
      const r = e.data
      if (r.id !== id) return
      if (typeof r.progress === 'number') {
        onProgress?.(r.progress)
        return
      }
      w.terminate()
      if (r.key) resolve(new Uint8Array(r.key))
      else reject(new Error(r.error ?? 'kdf failed'))
    }
    w.onerror = (e) => {
      w.terminate()
      reject(new Error(e.message || 'kdf worker failed'))
    }
    w.postMessage({ id, passphrase: normalisePassphrase(passphrase), salt: k.salt, m: k.m, t: k.t, p: k.p })
  })
}

/**
 * Derive the 32-byte key for a member key record. Throws on unusable
 * parameters. Uses the worker in the browser, else the async in-thread KDF.
 */
export async function deriveKey(passphrase: string, k: KdfParams, onProgress?: Progress): Promise<Uint8Array> {
  if (!validKdf(k)) throw new Error('unsupported kdf parameters')
  if (typeof Worker !== 'undefined' && typeof window !== 'undefined') {
    try {
      return await inWorker(passphrase, k, onProgress)
    } catch {
      // A worker the browser refused (CSP, old engine): do it here.
    }
  }
  return argon2InThread(passphrase, k, onProgress)
}
