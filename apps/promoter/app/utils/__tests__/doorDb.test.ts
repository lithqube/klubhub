import { describe, expect, it } from 'vitest'
import { DeviceKeyVault, DoorVault, memoryKV, newSessionKey, seal, unseal } from '../doorDb'

describe('doorDb crypto', () => {
  it('round-trips JSON through AES-GCM', async () => {
    const key = await newSessionKey()
    const value = { guests: [{ id: 'g1', name: 'Zoë Lindqvist', plus_n: 2 }], cursor: '42' }
    const s = await seal(key, 'bundle', value)
    expect(s.iv).toHaveLength(12)
    expect(new TextDecoder().decode(new Uint8Array(s.ct))).not.toContain('Lindqvist')
    expect(await unseal(key, 'bundle', s)).toEqual(value)
  })

  it('uses a non-extractable key', async () => {
    const key = await newSessionKey()
    expect(key.extractable).toBe(false)
    await expect(crypto.subtle.exportKey('raw', key)).rejects.toThrow()
  })

  it('refuses another key, another slot or a tampered ciphertext', async () => {
    const key = await newSessionKey()
    const s = await seal(key, 'queue', [1, 2, 3])
    await expect(unseal(await newSessionKey(), 'queue', s)).rejects.toThrow()
    await expect(unseal(key, 'bundle', s)).rejects.toThrow()
    const bad = new Uint8Array(s.ct.slice(0))
    bad[0]! ^= 1
    await expect(unseal(key, 'queue', { ...s, ct: bad.buffer })).rejects.toThrow()
  })
})

describe('DoorVault', () => {
  it('stores only ciphertext and the opaque key', async () => {
    const kv = memoryKV()
    const v = new DoorVault(kv)
    await v.create()
    await v.put('bundle', { name: 'Mara Weiss' })
    const raw = await kv.get('slot:bundle') as { ct: ArrayBuffer }
    expect(new TextDecoder().decode(new Uint8Array(raw.ct))).not.toContain('Mara')
    expect(await kv.get('session-key')).toBeInstanceOf(CryptoKey)
    expect(await v.get('bundle')).toEqual({ name: 'Mara Weiss' })
  })

  it('reopens after a reload with the stored key', async () => {
    const kv = memoryKV()
    const a = new DoorVault(kv)
    await a.create()
    await a.put('queue', { ops: [{ nonce: 'n1' }] })
    const b = new DoorVault(kv)
    expect(await b.get('queue')).toBeNull() // locked until opened
    expect(await b.open()).toBe(true)
    expect(await b.get('queue')).toEqual({ ops: [{ nonce: 'n1' }] })
  })

  it('wipe drops the key and the data; a new session cannot read old slots', async () => {
    const kv = memoryKV()
    const v = new DoorVault(kv)
    await v.create()
    await v.put('bundle', { a: 1 })
    const old = await kv.get('slot:bundle')
    await v.wipe()
    expect(v.isOpen).toBe(false)
    expect(await kv.get('slot:bundle')).toBeUndefined()
    expect(await new DoorVault(kv).open()).toBe(false)
    await v.create()
    await kv.set('slot:bundle', old)
    expect(await v.get('bundle')).toBeNull()
    await expect(new DoorVault(memoryKV()).put('x', 1)).rejects.toThrow('locked')
  })

  it('get returns null for a missing slot and when locked', async () => {
    const v = new DoorVault(memoryKV())
    expect(await v.get('nope')).toBeNull() // locked: no key
    await v.create()
    expect(await v.get('nope')).toBeNull() // open, but absent
  })

  it('open returns false when the stored key is not an object', async () => {
    const kv = memoryKV()
    await kv.set('session-key', 'not-a-key')
    const v = new DoorVault(kv)
    expect(await v.open()).toBe(false)
    expect(v.isOpen).toBe(false)
  })
})

describe('memoryKV', () => {
  it('supports get, set, del and clear', async () => {
    const kv = memoryKV()
    expect(await kv.get('a')).toBeUndefined()
    await kv.set('a', 1)
    await kv.set('b', { x: 2 })
    expect(await kv.get('a')).toBe(1)
    expect(await kv.get('b')).toEqual({ x: 2 })
    await kv.del('a')
    expect(await kv.get('a')).toBeUndefined()
    await kv.clear()
    expect(await kv.get('b')).toBeUndefined()
  })
})

describe('DeviceKeyVault', () => {
  const deviceId = 'door-1'
  const privateKey = new Uint8Array(32).map((_, i) => i)

  it('saves and loads the device private key', async () => {
    const v = new DeviceKeyVault(memoryKV())
    expect(await v.has(deviceId)).toBe(false)
    await v.save(deviceId, privateKey)
    expect(await v.has(deviceId)).toBe(true)
    const loaded = await v.load(deviceId)
    expect(loaded).toEqual(privateKey)
  })

  it('returns null for another device id', async () => {
    const v = new DeviceKeyVault(memoryKV())
    await v.save(deviceId, privateKey)
    expect(await v.load('door-2')).toBeNull()
    expect(await v.has('door-2')).toBe(false)
  })

  it('returns null when nothing is stored', async () => {
    const v = new DeviceKeyVault(memoryKV())
    expect(await v.load(deviceId)).toBeNull()
  })

  it('clear forgets the device', async () => {
    const v = new DeviceKeyVault(memoryKV())
    await v.save(deviceId, privateKey)
    await v.clear()
    expect(await v.has(deviceId)).toBe(false)
    expect(await v.load(deviceId)).toBeNull()
  })

  it('save replaces a previous device key', async () => {
    const v = new DeviceKeyVault(memoryKV())
    await v.save('door-1', privateKey)
    const other = new Uint8Array(32).fill(7)
    await v.save('door-2', other)
    expect(await v.load('door-1')).toBeNull()
    expect(await v.load('door-2')).toEqual(other)
  })
})
