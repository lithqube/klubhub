import { describe, expect, it } from 'vitest'
import { banMatches, banTokens, isSingleToken, namesMatch, openDoorBan, type DoorBanEntry } from '../doorBan'
import { b64url, randomBytes } from '../sealed/bytes'
import { encryptBan } from '../sealed/ban'
import { generateOsk, oskAad, wrapOsk } from '../sealed/keys'
import { x25519Keypair } from '../sealed/seal'

const entry = (name: string, extra: Partial<DoorBanEntry> = {}): DoorBanEntry => ({ id: name, name, reason: 'r', expires_at: '2099-01-01T00:00:00Z', ...extra })

describe('door ban matching', () => {
  it('folds accents and punctuation into tokens', () => {
    expect(banTokens('  Zoë  Brändt-Møller ')).toEqual(['zoe', 'brandt', 'moller'])
  })

  it('matches when every token of one name is in the other (either way round)', () => {
    expect(namesMatch('Viktor Brandt', 'viktor brandt')).toBe(true)
    expect(namesMatch('Viktor Brandt', 'Brandt, Viktor')).toBe(true)
    expect(namesMatch('Viktor Brändt', 'Viktor Brandt +1')).toBe(true)
    expect(namesMatch('Viktor Brandt', 'Viktor Emil Brandt')).toBe(true)
    expect(namesMatch('Viktor Emil Brandt', 'Viktor Brandt')).toBe(true)
    expect(namesMatch('Viktor Brandt', 'Viktor Brand')).toBe(false)
    expect(namesMatch('Viktor Brandt', 'Viktoria Brandt')).toBe(false)
    expect(namesMatch('Viktor Brandt', 'Anna Schulz')).toBe(false)
    expect(namesMatch('', 'Anna')).toBe(false)
  })

  it('needs at least two tokens in the shorter name for a subset match', () => {
    // One-word entries only match the same one word.
    expect(namesMatch('Ole', 'ole')).toBe(true)
    expect(namesMatch('Olé', 'OLE')).toBe(true)
    expect(namesMatch('Ole', 'Ole Petersen')).toBe(false)
    expect(namesMatch('Ole Petersen', 'Ole')).toBe(false)
    expect(namesMatch('Ole', 'Ola')).toBe(false)
    // Two or more tokens: the shorter is a subset of the longer, either way round.
    expect(namesMatch('Ole Petersen', 'Ole Jan Petersen')).toBe(true)
    expect(namesMatch('Ole Jan Petersen', 'Petersen Ole')).toBe(true)
    expect(namesMatch('Ole Petersen', 'Ole Jan')).toBe(false)
    // Repeated tokens count once.
    expect(namesMatch('Ole Ole', 'Ole Petersen')).toBe(false)
  })

  it('flags single-token names for the form', () => {
    expect(isSingleToken('Ole')).toBe(true)
    expect(isSingleToken('  Ole  ')).toBe(true)
    expect(isSingleToken('Ole Petersen')).toBe(false)
    expect(isSingleToken('Ole-Petersen')).toBe(false)
    expect(isSingleToken('')).toBe(false)
  })

  it('lists the possible matches for a guest, with email only when both carry one', () => {
    const list = [entry('Viktor Brandt'), entry('Mia Klein', { email: 'mia@example.org' }), entry('Ole')]
    expect(banMatches(list, 'viktor brandt').map(e => e.name)).toEqual(['Viktor Brandt'])
    expect(banMatches(list, 'Someone Else', 'MIA@example.org').map(e => e.name)).toEqual(['Mia Klein'])
    expect(banMatches(list, 'Ole Petersen').map(e => e.name)).toEqual([])
    expect(banMatches(list, 'OLE').map(e => e.name)).toEqual(['Ole'])
    expect(banMatches(null, 'Viktor Brandt')).toEqual([])
    expect(banMatches([], 'Viktor Brandt')).toEqual([])
  })
})

describe('openDoorBan', () => {
  it('opens the wrap with the device key, decrypts entries and drops expired ones', async () => {
    const dev = x25519Keypair()
    const osk = generateOsk()
    const now = Date.parse('2026-10-03T22:00:00Z')
    const sealed = {
      key_version: 3,
      wrap: await wrapOsk(osk, dev.publicKey, oskAad('t', 3, 'device', 'd1')),
      ban_entries: [
        { id: 'a', entry_sealed: await encryptBan(osk, 't', 'a', 3, { name: 'Viktor Brandt', reason: 'Fight' }), expires_at: '2026-12-01T00:00:00Z' },
        { id: 'b', entry_sealed: await encryptBan(osk, 't', 'b', 3, { name: 'Gone', reason: 'x' }), expires_at: '2026-10-01T00:00:00Z' },
        { id: 'c', entry_sealed: b64url(randomBytes(50)), expires_at: '2026-12-01T00:00:00Z' },
        // Encrypted for another id: the AAD binds it, so it does not open as "d".
        { id: 'd', entry_sealed: await encryptBan(osk, 't', 'a', 3, { name: 'Swapped', reason: 'x' }), expires_at: '2026-12-01T00:00:00Z' },
      ],
    }
    const r = await openDoorBan(sealed, dev.privateKey, 't', 'd1', now)
    expect(r.entries).toEqual([{ id: 'a', name: 'Viktor Brandt', reason: 'Fight', expires_at: '2026-12-01T00:00:00Z' }])
    expect(r.unreadable).toBe(2)
    await expect(openDoorBan(sealed, x25519Keypair().privateKey, 't', 'd1', now)).rejects.toThrow()
    await expect(openDoorBan(sealed, dev.privateKey, 't', 'd2', now)).rejects.toThrow()
  })
})
