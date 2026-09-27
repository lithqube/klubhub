import { describe, expect, it } from 'vitest'
import { signInAgainRoute, stepUpLoginText } from '../privacy'
import { banErrorText, progressText, sealedErrorText, securityBlock, securityRefusal } from '../sealedText'

describe('sealed copy', () => {
  it('says what an owner key action is missing before it is tried', () => {
    const now = Date.parse('2026-09-27T12:00:00Z')
    expect(securityBlock(null, now)).toBeNull()
    expect(securityBlock({ mfa: false, auth_time: '2026-09-27T11:59:00Z' }, now)).toBe('mfa')
    expect(securityBlock({ mfa: true, auth_time: '2026-09-27T11:50:00Z' }, now)).toBeNull()
    expect(securityBlock({ mfa: true, auth_time: '2026-09-27T11:40:00Z' }, now)).toBe('stale')
  })

  it('maps security.manage refusals to the 2FA or sign-in-again notice', () => {
    expect(securityRefusal({ error: 'mfa_required' })).toBe('mfa')
    expect(securityRefusal({ error: 'reauthentication_required' })).toBe('stale')
    expect(securityRefusal({ error: 'kit_mismatch' })).toBeNull()
    expect(securityRefusal(null)).toBeNull()
  })

  it('has one plain sentence per refusal, with a safe default', () => {
    expect(sealedErrorText({ error: 'wrong_passphrase' })).toMatch(/does not unlock/)
    expect(sealedErrorText({ error: 'kit_length', count: 49 })).toContain('you typed 49')
    expect(sealedErrorText({ error: 'unreadable_entries', count: 1 })).toMatch(/1 ban list entry does not open/)
    expect(sealedErrorText({ error: 'unreadable_entries', count: 3 })).toMatch(/3 ban list entries do not open/)
    expect(sealedErrorText({ error: 'public_key_mismatch' })).toMatch(/recovering with the kit/)
    expect(sealedErrorText({ error: 'something_new' })).toBe('Something went wrong. Try again.')
    expect(sealedErrorText(null)).toBe('')
    expect(banErrorText({ error: 'key_version_stale' })).toMatch(/RELOAD AND RETRY/)
    expect(banErrorText({ error: 'invalid', field: 'expires_at' })).toMatch(/3 years/)
    expect(banErrorText({ error: 'ban_entry_exists' })).toMatch(/already saved/)
  })

  it('formats KDF progress', () => {
    expect(progressText(0.424)).toBe('42 %')
    expect(progressText(2)).toBe('100 %')
    expect(progressText(null)).toBe('')
  })

  it('SIGN IN AGAIN for key management comes back to the section', () => {
    expect(signInAgainRoute('/settings#sealed', 'sealed')).toEqual({ path: '/login', query: { next: '/settings#sealed', why: 'sealed' } })
    expect(stepUpLoginText('sealed')).toMatch(/encryption keys/)
  })
})
