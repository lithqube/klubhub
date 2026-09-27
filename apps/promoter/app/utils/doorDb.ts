/**
 * Door device cache (P2.3, "Door device security"): the bundle holds
 * decrypted names, so everything the door keeps is sealed with AES-GCM
 * under a NON-EXTRACTABLE key created per door session. The CryptoKey
 * object itself is stored in IndexedDB (structured clone keeps it opaque:
 * script can use it but never read its bytes). Logout, session expiry and
 * event end call wipe(), which drops the key with the data.
 *
 * No dependency: a tiny promise wrapper over one IndexedDB object store,
 * behind a KV interface so tests (jsdom has no IndexedDB) use memoryKV().
 */

export interface KV {
  get(key: string): Promise<unknown>
  set(key: string, value: unknown): Promise<void>
  del(key: string): Promise<void>
  clear(): Promise<void>
}

export const DOOR_DB = 'klubhub-door'
const STORE = 'kv'

function req<T>(r: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    r.onsuccess = () => resolve(r.result)
    r.onerror = () => reject(r.error)
  })
}

/** IndexedDB-backed KV (one database, one object store). */
export function idbKV(name = DOOR_DB): KV {
  let db: Promise<IDBDatabase> | null = null
  const open = () => (db ??= new Promise((resolve, reject) => {
    const r = indexedDB.open(name, 1)
    r.onupgradeneeded = () => r.result.createObjectStore(STORE)
    r.onsuccess = () => resolve(r.result)
    r.onerror = () => reject(r.error)
  }))
  const tx = async (mode: IDBTransactionMode) => (await open()).transaction(STORE, mode).objectStore(STORE)
  return {
    get: async key => req((await tx('readonly')).get(key)),
    set: async (key, value) => { await req((await tx('readwrite')).put(value, key)) },
    del: async (key) => { await req((await tx('readwrite')).delete(key)) },
    clear: async () => { await req((await tx('readwrite')).clear()) },
  }
}

/** In-memory KV (tests, or a browser without IndexedDB: nothing survives a reload). */
export function memoryKV(): KV {
  const m = new Map<string, unknown>()
  return {
    get: async key => m.get(key),
    set: async (key, value) => { m.set(key, value) },
    del: async (key) => { m.delete(key) },
    clear: async () => { m.clear() },
  }
}

export interface Sealed {
  v: 1
  iv: Uint8Array
  ct: ArrayBuffer
}

const enc = new TextEncoder()
const dec = new TextDecoder()

/** A fresh AES-GCM-256 key that can encrypt and decrypt but never be exported. */
export function newSessionKey(): Promise<CryptoKey> {
  return globalThis.crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt'])
}

/** JSON-serialise and encrypt value; `slot` is bound as additional data so ciphertexts cannot be swapped between slots. */
export async function seal(key: CryptoKey, slot: string, value: unknown): Promise<Sealed> {
  const iv = globalThis.crypto.getRandomValues(new Uint8Array(12))
  const ct = await globalThis.crypto.subtle.encrypt(
    { name: 'AES-GCM', iv: iv as BufferSource, additionalData: enc.encode(slot) as BufferSource }, key, enc.encode(JSON.stringify(value)) as BufferSource)
  return { v: 1, iv, ct }
}

/** Decrypt and parse; throws when the key, slot or ciphertext does not match. */
export async function unseal<T>(key: CryptoKey, slot: string, s: Sealed): Promise<T> {
  const pt = await globalThis.crypto.subtle.decrypt(
    { name: 'AES-GCM', iv: s.iv as BufferSource, additionalData: enc.encode(slot) as BufferSource }, key, s.ct)
  return JSON.parse(dec.decode(pt)) as T
}

const KEY_SLOT = 'session-key'

/** Sealed slots in a KV, under one per-session key. */
export class DoorVault {
  private key: CryptoKey | null = null
  constructor(private readonly kv: KV) {}

  /** Start a new session: wipe everything and create a fresh key. */
  async create(): Promise<void> {
    await this.kv.clear()
    this.key = await newSessionKey()
    await this.kv.set(KEY_SLOT, this.key)
  }

  /** Reopen the session key after a reload; false when there is none. */
  async open(): Promise<boolean> {
    const k = await this.kv.get(KEY_SLOT)
    this.key = k && typeof k === 'object' ? (k as CryptoKey) : null
    return !!this.key
  }

  get isOpen(): boolean {
    return !!this.key
  }

  async put(slot: string, value: unknown): Promise<void> {
    if (!this.key) throw new Error('door vault is locked')
    await this.kv.set(`slot:${slot}`, await seal(this.key, slot, value))
  }

  /** The slot's value, or null when absent or unreadable (wrong key, tampered). */
  async get<T>(slot: string): Promise<T | null> {
    if (!this.key) return null
    const s = (await this.kv.get(`slot:${slot}`)) as Sealed | undefined
    if (!s) return null
    try {
      return await unseal<T>(this.key, slot, s)
    } catch {
      return null
    }
  }

  /** Drop the key and every slot. */
  async wipe(): Promise<void> {
    this.key = null
    await this.kv.clear()
  }
}

// ---------------------------------------------------------------- device key (P2.6)

/**
 * The door device's X25519 private key (it opens the ban list offline).
 * It must outlive door sessions — logout and session wipes clear the
 * session vault above — so it sits in a vault of its own: a separate
 * IndexedDB database, sealed the same way under its own non-extractable
 * AES-GCM key. Only "forget this device" (or revoking it here) clears it.
 */
export const DEVICE_DB = 'klubhub-door-device'
const DEVICE_SLOT = 'device-key'

interface DeviceSecret {
  device_id: string
  private_key: number[]
}

export class DeviceKeyVault {
  private readonly vault: DoorVault
  constructor(kv: KV) {
    this.vault = new DoorVault(kv)
  }

  /** Replace whatever is stored with this device's private key. */
  async save(deviceId: string, privateKey: Uint8Array): Promise<void> {
    await this.vault.create()
    const s: DeviceSecret = { device_id: deviceId, private_key: [...privateKey] }
    await this.vault.put(DEVICE_SLOT, s)
    s.private_key.fill(0)
  }

  /** The private key for deviceId, or null (none stored, another device's, unreadable). */
  async load(deviceId: string): Promise<Uint8Array | null> {
    if (!(await this.vault.open())) return null
    const s = await this.vault.get<DeviceSecret>(DEVICE_SLOT)
    if (!s || s.device_id !== deviceId || !Array.isArray(s.private_key) || s.private_key.length !== 32) return null
    return Uint8Array.from(s.private_key)
  }

  async has(deviceId: string): Promise<boolean> {
    const k = await this.load(deviceId)
    k?.fill(0)
    return !!k
  }

  async clear(): Promise<void> {
    await this.vault.wipe()
  }
}
