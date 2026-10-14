import { describe, expect, it } from 'vitest'
import type { KdfParams } from '~/types/sealed'
import { fromB64url } from '../bytes'
import { argon2InThread, deriveKey, KDF_ALG, KDF_DEFAULTS, newKdfParams, normalisePassphrase, SALT_BYTES, validKdf } from '../kdf'

/** Tiny but valid parameters: fast enough for a unit test. */
const tiny: KdfParams = { alg: KDF_ALG, m: 8, t: 1, p: 1, salt: 'AAAAAAAAAAAAAAAAAAAAAA' }

describe('newKdfParams', () => {
  it('produces production-cost params with a fresh 16-byte salt', () => {
    const k = newKdfParams()
    expect(k.alg).toBe(KDF_ALG)
    expect(k.m).toBe(KDF_DEFAULTS.m)
    expect(k.t).toBe(KDF_DEFAULTS.t)
    expect(k.p).toBe(KDF_DEFAULTS.p)
    expect(fromB64url(k.salt)).toHaveLength(SALT_BYTES)
  })

  it('generates a different salt each time', () => {
    expect(newKdfParams().salt).not.toBe(newKdfParams().salt)
  })
})

describe('validKdf', () => {
  it('accepts the production defaults and tiny test params', () => {
    expect(validKdf(newKdfParams())).toBe(true)
    expect(validKdf(tiny)).toBe(true)
  })

  it('rejects a wrong algorithm, missing fields and non-integers', () => {
    expect(validKdf(null)).toBe(false)
    expect(validKdf({})).toBe(false)
    expect(validKdf({ ...tiny, alg: 'argon2i' })).toBe(false)
    expect(validKdf({ ...tiny, m: 8.5 })).toBe(false)
    expect(validKdf({ ...tiny, t: 0 })).toBe(false)
    expect(validKdf({ ...tiny, p: 0 })).toBe(false)
  })

  it('rejects parameters outside the anti-DoS bounds', () => {
    expect(validKdf({ ...tiny, m: 7 })).toBe(false) // below the 8 KiB floor
    expect(validKdf({ ...tiny, m: 262_145 })).toBe(false) // above MAX.m
    expect(validKdf({ ...tiny, t: 11 })).toBe(false) // above MAX.t
    expect(validKdf({ ...tiny, p: 5 })).toBe(false) // above MAX.p
    expect(validKdf({ ...tiny, m: 8, p: 2 })).toBe(false) // m < 8 * p lanes
  })

  it('rejects a salt that is not 16 bytes or not base64url', () => {
    expect(validKdf({ ...tiny, salt: 'AAAA' })).toBe(false)
    expect(validKdf({ ...tiny, salt: 'not base64url!!' })).toBe(false)
    expect(validKdf({ ...tiny, salt: undefined as unknown as string })).toBe(false)
  })
})

describe('normalisePassphrase', () => {
  it('applies NFKC so composed and decomposed input match', () => {
    expect(normalisePassphrase('café')).toBe(normalisePassphrase('café'))
    expect(normalisePassphrase('ＡＢＣ')).toBe('ABC') // fullwidth → ASCII
  })
})

describe('argon2InThread', () => {
  it('derives a deterministic 32-byte key', async () => {
    const a = await argon2InThread('correct horse', tiny)
    const b = await argon2InThread('correct horse', tiny)
    expect(a).toHaveLength(32)
    expect(a).toEqual(b)
  })

  it('differs per passphrase and per salt', async () => {
    const base = await argon2InThread('correct horse', tiny)
    const otherPass = await argon2InThread('correct horse!', tiny)
    const otherSalt = await argon2InThread('correct horse', { ...tiny, salt: 'BBBBBBBBBBBBBBBBBBBBBB' })
    expect(base).not.toEqual(otherPass)
    expect(base).not.toEqual(otherSalt)
  })

  it('reports progress fractions in [0, 1]', async () => {
    const seen: number[] = []
    await argon2InThread('pw', tiny, (f) => seen.push(f))
    expect(seen.length).toBeGreaterThan(0)
    for (const f of seen) expect(f).toBeGreaterThanOrEqual(0)
    for (const f of seen) expect(f).toBeLessThanOrEqual(1)
  })
})

describe('deriveKey', () => {
  it('throws on unusable parameters without calling the KDF', async () => {
    await expect(deriveKey('pw', { ...tiny, alg: 'nope' as KdfParams['alg'] })).rejects.toThrow('unsupported kdf parameters')
  })

  it('derives in-thread where there is no Worker (jsdom)', async () => {
    const key = await deriveKey('correct horse', tiny)
    expect(key).toHaveLength(32)
    expect(key).toEqual(await argon2InThread('correct horse', tiny))
  })
})
