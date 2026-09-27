import { pbkdf2Sync, randomBytes } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import { b64decode, b64encode, constantTimeEqual, pbkdf2Sha256, verifyManagerPin } from '../doorPin'

// Vector computed here with Node's PBKDF2 (what the Go API's crypto/pbkdf2
// produces too), then checked through WebCrypto. Low iterations keep the
// test fast; the bundle uses 210 000.
const salt = randomBytes(16)
const iterations = 1000
const verifier = (pin: string) => ({ salt: salt.toString('base64'), iterations, hash: pbkdf2Sync(pin, salt, iterations, 32, 'sha256').toString('base64') })

describe('doorPin', () => {
  it('derives the same PBKDF2-SHA256 bytes as Node', async () => {
    const got = await pbkdf2Sha256('246810', new Uint8Array(salt), iterations)
    expect(Buffer.from(got).toString('hex')).toBe(pbkdf2Sync('246810', salt, iterations, 32, 'sha256').toString('hex'))
  })

  it('matches the RFC 7914 PBKDF2-HMAC-SHA256 test vector', async () => {
    const got = await pbkdf2Sha256('passwd', new TextEncoder().encode('salt'), 1, 64)
    expect(Buffer.from(got).toString('hex')).toBe(
      '55ac046e56e3089fec1691c22544b605f94185216dde0465e68b9d57c20dacbc49ca9cccf179b645991664b39d77ef317c71b845b1e30bd509112041d3a19783')
  })

  it('verifies the right manager PIN and refuses others', async () => {
    const v = verifier('246810')
    expect(await verifyManagerPin('246810', v)).toBe(true)
    expect(await verifyManagerPin('246811', v)).toBe(false)
    expect(await verifyManagerPin('24681', v)).toBe(false)
    expect(await verifyManagerPin('abcdef', v)).toBe(false)
    expect(await verifyManagerPin('246810', null)).toBe(false)
    expect(await verifyManagerPin('246810', { ...v, salt: '%%%' })).toBe(false)
  })

  it('compares in constant time and round-trips base64', () => {
    const a = new Uint8Array([1, 2, 3])
    expect(constantTimeEqual(a, new Uint8Array([1, 2, 3]))).toBe(true)
    expect(constantTimeEqual(a, new Uint8Array([1, 2, 4]))).toBe(false)
    expect(constantTimeEqual(a, new Uint8Array([1, 2]))).toBe(false)
    const bytes = new Uint8Array(randomBytes(32))
    expect(b64decode(b64encode(bytes))).toEqual(bytes)
    expect(b64encode(bytes)).toBe(Buffer.from(bytes).toString('base64'))
  })
})
