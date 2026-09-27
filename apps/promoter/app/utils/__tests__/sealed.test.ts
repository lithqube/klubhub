import { describe, expect, it } from 'vitest'
import type { BanEntry, KdfParams } from '~/types/sealed'
import { b64url, concat, fromB64url, randomBytes, utf8 } from '../sealed/bytes'
import { argon2InThread, deriveKey, KDF_DEFAULTS, newKdfParams, validKdf } from '../sealed/kdf'
import {
  banAad, base32, createMemberKey, fingerprint, formatFingerprint, fromBase32, generateOsk, generateRecoveryKit, groupMatches, kitFileText,
  kitGroups, memberIdCandidates, normaliseKit, oskAad, parseKit, passphraseStrength, pickChallenge, recoveryKeypair, unlockMemberKey, unwrapOsk,
  wrapOsk,
} from '../sealed/keys'
import { aesOpen, aesSeal, AES_MIN, openSealed, seal, SEAL_MIN, SealedError, x25519Keypair } from '../sealed/seal'
import {
  customExpiry, decryptBan, encryptBan, maxExpiryDate, minExpiryDate, presetExpiry, searchBan, sortBan, tidyPlain, validatePlain,
} from '../sealed/ban'

/** Tiny Argon2id parameters: the tests check the construction, not the cost. */
const tiny = (): KdfParams => ({ alg: 'argon2id', m: 8, t: 1, p: 1, salt: b64url(randomBytes(16)) })
const fastDerive = (pass: string, k: KdfParams) => argon2InThread(pass, k)

const rejects = async (p: Promise<unknown>, code: 'unreadable' | 'malformed') => {
  await expect(p).rejects.toBeInstanceOf(SealedError)
  await expect(p).rejects.toMatchObject({ code })
}

describe('bytes', () => {
  it('base64url round-trips without padding and accepts padded / standard input', () => {
    for (let n = 0; n < 40; n++) {
      const b = randomBytes(n)
      const s = b64url(b)
      expect(s).not.toMatch(/[=+/]/)
      expect(fromB64url(s)).toEqual(b)
    }
    expect(fromB64url('-_8=')).toEqual(fromB64url('+/8'))
    expect(() => fromB64url('a')).toThrow()
    expect(() => fromB64url('ab$c')).toThrow()
  })
})

describe('seal / open (X25519 + HKDF + AES-GCM)', () => {
  it('round-trips with the byte layout 0x01 ‖ eph_pub ‖ nonce ‖ ct+tag', async () => {
    const r = x25519Keypair()
    const pt = utf8('the org sealed key')
    const blob = await seal(r.publicKey, pt, 'ctx')
    expect(blob[0]).toBe(1)
    expect(blob.length).toBe(1 + 32 + 12 + pt.length + 16)
    expect(SEAL_MIN).toBe(61)
    expect(await openSealed(r.privateKey, blob, 'ctx')).toEqual(pt)
    // A fresh ephemeral key and nonce each time.
    expect(b64url(await seal(r.publicKey, pt, 'ctx'))).not.toBe(b64url(blob))
  })

  it('fails with the wrong AAD, the wrong key, a flipped byte or a wrong version', async () => {
    const r = x25519Keypair()
    const other = x25519Keypair()
    const blob = await seal(r.publicKey, utf8('secret'), 'ctx-a')
    await rejects(openSealed(r.privateKey, blob, 'ctx-b'), 'unreadable')
    await rejects(openSealed(other.privateKey, blob, 'ctx-a'), 'unreadable')
    for (const i of [1, 20, 40, blob.length - 1]) {
      const t = blob.slice()
      t[i]! ^= 0x01
      await rejects(openSealed(r.privateKey, t, 'ctx-a'), 'unreadable')
    }
    const v2 = blob.slice()
    v2[0] = 2
    await rejects(openSealed(r.privateKey, v2, 'ctx-a'), 'malformed')
    await rejects(openSealed(r.privateKey, blob.slice(0, 40), 'ctx-a'), 'malformed')
  })

  it('refuses a low-order public key instead of sealing to it', async () => {
    await rejects(seal(new Uint8Array(32), utf8('x'), 'ctx'), 'malformed')
  })

  it('aesSeal is 0x01 ‖ nonce ‖ ct+tag and binds the AAD', async () => {
    const k = randomBytes(32)
    const blob = await aesSeal(k, utf8('hello'), 'a')
    expect(blob[0]).toBe(1)
    expect(blob.length).toBe(AES_MIN + 5)
    expect(new TextDecoder().decode(await aesOpen(k, blob, 'a'))).toBe('hello')
    await rejects(aesOpen(k, blob, 'b'), 'unreadable')
    await rejects(aesOpen(randomBytes(32), blob, 'a'), 'unreadable')
    const t = blob.slice()
    t[5]! ^= 0x80
    await rejects(aesOpen(k, t, 'a'), 'unreadable')
  })
})

describe('Argon2id KDF', () => {
  it('uses the contract parameters for new keys', () => {
    const k = newKdfParams()
    expect(k).toMatchObject({ alg: 'argon2id', m: 65_536, t: 3, p: 1 })
    expect(KDF_DEFAULTS).toEqual({ m: 65_536, t: 3, p: 1 })
    expect(fromB64url(k.salt)).toHaveLength(16)
    expect(validKdf(k)).toBe(true)
  })

  it('is deterministic per passphrase and salt, NFKC-normalised, and different per salt', async () => {
    const k = tiny()
    const a = await argon2InThread('correct horse battery', k)
    expect(a).toHaveLength(32)
    expect(await argon2InThread('correct horse battery', k)).toEqual(a)
    // "ﬁ" (U+FB01) is "fi" after NFKC.
    expect(await argon2InThread('ﬁne passphrase', k)).toEqual(await argon2InThread('fine passphrase', k))
    expect(await argon2InThread('correct horse battery', tiny())).not.toEqual(a)
  })

  it('matches a reference Argon2id output for fixed inputs', async () => {
    // Same value as Node's native crypto.argon2Sync('argon2id', …); pinned so a library
    // upgrade that changed the output (existing keys would stop unlocking) is caught.
    const k: KdfParams = { alg: 'argon2id', m: 8, t: 1, p: 1, salt: b64url(utf8('0123456789abcdef')) }
    expect(b64url(await argon2InThread('password', k))).toBe('dxM42BlXPGcRaznheIro4EsOsM-d-7_i5tdGzz5GT8c')
  })

  it('refuses unknown or extreme parameters', async () => {
    expect(validKdf({ ...tiny(), alg: 'scrypt' })).toBe(false)
    expect(validKdf({ ...tiny(), m: 10_000_000 })).toBe(false)
    expect(validKdf({ ...tiny(), t: 0 })).toBe(false)
    expect(validKdf({ ...tiny(), salt: b64url(randomBytes(8)) })).toBe(false)
    await expect(deriveKey('x', { ...tiny(), t: 0 })).rejects.toThrow()
  })
})

describe('member key', () => {
  it('creates and unlocks with the passphrase; a wrong passphrase is unreadable', async () => {
    const created = await createMemberKey('long enough passphrase', { kdf: tiny(), derive: fastDerive })
    expect(fromB64url(created.record.public_key)).toEqual(created.publicKey)
    expect(created.record.kdf.m).toBe(8)
    const priv = await unlockMemberKey('long enough passphrase', created.record, { derive: fastDerive })
    expect(priv).toEqual(created.privateKey)
    await rejects(unlockMemberKey('wrong passphrase!!', created.record, { derive: fastDerive }), 'unreadable')
  })

  it('binds the private key to its public key', async () => {
    const a = await createMemberKey('long enough passphrase', { kdf: tiny(), derive: fastDerive })
    const b = await createMemberKey('long enough passphrase', { kdf: a.record.kdf, derive: fastDerive })
    // a's ciphertext presented with b's public key does not open.
    await rejects(unlockMemberKey('long enough passphrase', { ...a.record, public_key: b.record.public_key }, { derive: fastDerive }), 'unreadable')
    await rejects(unlockMemberKey('long enough passphrase', { ...a.record, private_sealed: 'nope' }, { derive: fastDerive }), 'malformed')
  })

  it('member recipient ids: the uuid without "local:", then the subject', () => {
    expect(memberIdCandidates('local:abc')).toEqual(['abc', 'local:abc'])
    expect(memberIdCandidates('zitadel:42')).toEqual(['zitadel:42'])
  })
})

describe('org sealed key wraps', () => {
  it('wraps to a member and opens only with the same context', async () => {
    const osk = generateOsk()
    const m = x25519Keypair()
    const aad = oskAad('t1', 3, 'member', 'u1')
    expect(aad).toBe('klubhub-osk|t1|3|member|u1')
    expect(oskAad('t1', 1, 'recovery', null)).toBe('klubhub-osk|t1|1|recovery|')
    const wrap = await wrapOsk(osk, b64url(m.publicKey), aad)
    expect(await unwrapOsk(m.privateKey, wrap, aad)).toEqual(osk)
    await rejects(unwrapOsk(m.privateKey, wrap, oskAad('t1', 3, 'member', 'u2')), 'unreadable')
    await rejects(unwrapOsk(m.privateKey, wrap, oskAad('t1', 4, 'member', 'u1')), 'unreadable')
    await rejects(unwrapOsk(m.privateKey, wrap, oskAad('t2', 3, 'member', 'u1')), 'unreadable')
    await rejects(unwrapOsk(m.privateKey, wrap, oskAad('t1', 3, 'device', 'u1')), 'unreadable')
    await rejects(unwrapOsk(m.privateKey, '%%', aad), 'malformed')
  })

  it('refuses a wrap that does not hold 32 bytes', async () => {
    const m = x25519Keypair()
    const w = b64url(await seal(m.publicKey, randomBytes(16), 'x'))
    await rejects(unwrapOsk(m.privateKey, w, 'x'), 'malformed')
  })
})

describe('recovery kit', () => {
  it('is 8 groups of 7 base32 characters that parse back to the secret', async () => {
    const kit = await generateRecoveryKit()
    expect(kit.groups).toHaveLength(8)
    for (const g of kit.groups) expect(g).toMatch(/^[A-Z2-7]{7}$/)
    const parsed = parseKit(kit.groups.join(' '))
    expect(parsed.error).toBeNull()
    expect(parsed.secret).toEqual(kit.secret)
  })

  it('parsing tolerates spaces, case, dashes, dots and 0/1/8 look-alikes', () => {
    const secret = randomBytes(32)
    const g = kitGroups(secret)
    const variants = [
      g.join(''), g.join('-'), g.join(' ').toLowerCase(), `  ${g.slice(0, 4).join(' ')}\n${g.slice(4).join(' ')}  `, g.join('.'),
      g.join(' ').replace(/O/g, '0').replace(/I/g, '1').replace(/B/g, '8'),
    ]
    for (const v of variants) expect(parseKit(v).secret).toEqual(secret)
    expect(normaliseKit('ab-c d.e')).toBe('ABCDE')
    expect(groupMatches(g[2]!.toLowerCase(), g[2]!)).toBe(true)
    expect(groupMatches(g[2]!.slice(1), g[2]!)).toBe(false)
  })

  it('reports length, character and checksum problems', () => {
    const g = kitGroups(randomBytes(32))
    expect(parseKit(g.slice(0, 7).join(' '))).toMatchObject({ error: 'length', count: 49 })
    expect(parseKit(`${g.join('')}!`)).toMatchObject({ error: 'chars' })
    // One typo in the secret part is caught by the checksum.
    const s = g.join('')
    const typo = (s[3] === 'A' ? 'B' : 'A')
    expect(parseKit(s.slice(0, 3) + typo + s.slice(4))).toMatchObject({ error: 'checksum' })
  })

  it('base32 encodes RFC 4648 vectors', () => {
    expect(base32(utf8('foobar'))).toBe('MZXW6YTBOI')
    expect(new TextDecoder().decode(fromBase32('MZXW6YTBOI'))).toBe('foobar')
  })

  it('derives the same keypair from the same secret, and the fingerprint is 8 bytes of hex', async () => {
    const secret = randomBytes(32)
    const a = await recoveryKeypair(secret)
    const b = await recoveryKeypair(secret.slice())
    expect(a.privateKey).toEqual(b.privateKey)
    expect(a.publicKey).toEqual(b.publicKey)
    expect(fingerprint(a.publicKey)).toMatch(/^[0-9a-f]{16}$/)
    expect(fingerprint(a.publicKey)).toBe(fingerprint(b.publicKey))
    expect((await recoveryKeypair(randomBytes(32))).publicKey).not.toEqual(a.publicKey)
    expect(formatFingerprint('3f9a01c277b0e4d1')).toBe('3F9A 01C2 77B0 E4D1')
  })

  it('opens the OSK wrapped to the recovery key after re-deriving it from the typed kit', async () => {
    const kit = await generateRecoveryKit()
    const osk = generateOsk()
    const aad = oskAad('t1', 1, 'recovery', null)
    const wrap = await wrapOsk(osk, kit.publicKey, aad)
    const typed = parseKit(kit.groups.join('-').toLowerCase())
    const kp = await recoveryKeypair(typed.secret!)
    expect(fingerprint(kp.publicKey)).toBe(kit.fingerprint)
    expect(await unwrapOsk(kp.privateKey, wrap, aad)).toEqual(osk)
  })

  it('picks two different groups for the re-type check', () => {
    for (let i = 0; i < 200; i++) {
      const [a, b] = pickChallenge()
      expect(a).toBeLessThan(b)
      expect(a).toBeGreaterThanOrEqual(0)
      expect(b).toBeLessThan(8)
    }
    expect(pickChallenge(() => 0.999)).toEqual([6, 7])
    expect(pickChallenge(() => 0)).toEqual([0, 1])
  })

  it('writes a kit file with the groups and the fingerprint', () => {
    const txt = kitFileText({ groups: ['AAAAAAA', 'BBBBBBB', 'CCCCCCC', 'DDDDDDD', 'EEEEEEE', 'FFFFFFF', 'GGGGGGG', 'HHHHHHH'], fingerprint: '3f9a01c277b0e4d1', org: 'Nachtwerk', createdAt: '2026-09-27' })
    expect(txt).toContain('AAAAAAA BBBBBBB CCCCCCC DDDDDDD')
    expect(txt).toContain('EEEEEEE FFFFFFF GGGGGGG HHHHHHH')
    expect(txt).toContain('3F9A 01C2 77B0 E4D1')
    expect(txt).toContain('Nachtwerk')
  })
})

describe('passphrase strength', () => {
  it('needs 12 characters and rewards length and variety', () => {
    expect(passphraseStrength('short')).toBe('short')
    expect(passphraseStrength('aaaaaaaaaaaaaaaa')).toBe('weak')
    expect(passphraseStrength('abcdefghijkl')).toBe('weak')
    expect(passphraseStrength('abcdefgh1234')).toBe('ok')
    expect(passphraseStrength('purple tiger river moon')).toBe('strong')
  })
})

describe('ban entries', () => {
  it('round-trips under the OSK and binds tenant, id and version', async () => {
    const osk = generateOsk()
    const p = { name: '  Viktor   Brandt ', reason: 'Fight at the bar', email: ' V@EXAMPLE.org ', note: '' }
    const s = await encryptBan(osk, 't1', 'b1', 2, p)
    const blob = fromB64url(s)
    expect(blob[0]).toBe(1)
    expect(await decryptBan(osk, 't1', 'b1', 2, s)).toEqual({ name: 'Viktor Brandt', reason: 'Fight at the bar', email: 'v@example.org' })
    expect(banAad('t1', 'b1', 2)).toBe('klubhub-ban|t1|b1|2')
    await rejects(decryptBan(osk, 't1', 'b2', 2, s), 'unreadable')
    await rejects(decryptBan(osk, 't1', 'b1', 3, s), 'unreadable')
    await rejects(decryptBan(osk, 't2', 'b1', 2, s), 'unreadable')
    await rejects(decryptBan(generateOsk(), 't1', 'b1', 2, s), 'unreadable')
    const t = blob.slice()
    t[t.length - 3]! ^= 1
    await rejects(decryptBan(osk, 't1', 'b1', 2, b64url(t)), 'unreadable')
  })

  it('refuses a ciphertext that is not an entry', async () => {
    const osk = generateOsk()
    const bad = b64url(await aesSeal(osk, utf8('{"name":1}'), banAad('t', 'i', 1)))
    await rejects(decryptBan(osk, 't', 'i', 1, bad), 'malformed')
    const notJson = b64url(await aesSeal(osk, utf8('nope'), banAad('t', 'i', 1)))
    await rejects(decryptBan(osk, 't', 'i', 1, notJson), 'malformed')
    await rejects(decryptBan(osk, 't', 'i', 1, b64url(concat(Uint8Array.of(1), randomBytes(5)))), 'malformed')
  })

  it('requires name and reason; email optional but valid', () => {
    expect(validatePlain({ name: '', reason: '' })).toEqual({ name: expect.any(String), reason: expect.any(String) })
    expect(validatePlain({ name: 'A', reason: 'B', email: 'nope' })).toEqual({ email: expect.any(String) })
    expect(validatePlain({ name: 'A', reason: 'B', email: 'a@b.co' })).toEqual({})
    expect(tidyPlain({ name: 'A', reason: 'B', email: '  ', note: ' ' })).toEqual({ name: 'A', reason: 'B' })
  })

  it('expiry presets and custom dates stay within a day and 3 years', () => {
    const now = Date.parse('2026-09-27T12:00:00Z')
    expect(presetExpiry('30d', now)).toBe('2026-10-27T12:00:00.000Z')
    expect(presetExpiry('6m', now)).toBe('2027-03-27T12:00:00.000Z')
    expect(presetExpiry('1y', now)).toBe('2027-09-27T12:00:00.000Z')
    expect(presetExpiry('6m', Date.parse('2026-08-31T12:00:00Z'))).toBe('2027-02-28T12:00:00.000Z')
    expect(minExpiryDate(now)).toBe('2026-09-29')
    expect(maxExpiryDate(now)).toBe('2029-09-26')
    expect(customExpiry('2026-09-28', now).error).toMatch(/later date/)
    expect(customExpiry('2026-09-29', now)).toEqual({ at: '2026-09-29T23:59:00.000Z', error: null })
    expect(customExpiry('2029-09-26', now).error).toBeNull()
    expect(customExpiry('2029-09-27', now).error).toMatch(/3 years/)
    expect(customExpiry('', now).error).toMatch(/Pick a date/)
  })

  it('searches names and emails accent-folded, prefix per token', () => {
    const e = (id: string, name: string, email?: string): BanEntry => ({ id, key_version: 1, expires_at: '', created_at: '', updated_at: '', plain: { name, reason: 'r', email } })
    const list = [e('1', 'Zoë Brändt'), e('2', 'Max Power', 'max@power.example'), { ...e('3', 'x'), plain: null }]
    expect(searchBan(list, 'zoe bra').map(x => x.id)).toEqual(['1'])
    expect(searchBan(list, 'power.ex').map(x => x.id)).toEqual(['2'])
    expect(searchBan(list, '').map(x => x.id)).toEqual(['1', '2', '3'])
    expect(sortBan(list).map(x => x.id)).toEqual(['2', '1', '3'])
  })
})
