import { describe, expect, it } from 'vitest'
import { DoorVault, memoryKV, newSessionKey, seal, unseal } from '../doorDb'

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
})
