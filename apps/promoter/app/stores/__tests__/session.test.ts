import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useSessionStore } from '../session'

const fetchMock = vi.fn()
vi.stubGlobal('$fetch', fetchMock)

const owner = { sub: 'local:1', org_id: 'o', roles: ['owner'], mfa: false, auth_time: '' }

describe('useSessionStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
  })

  it('logs in with the CSRF header and loads the session', async () => {
    fetchMock.mockResolvedValueOnce({ totp_enrolled: false }).mockResolvedValueOnce(owner)
    const s = useSessionStore()
    expect(await s.login('a@b.c', 'long passphrase')).toBeNull()
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/auth/login')
    expect(opts.headers['X-KlubHub-CSRF']).toBe('1')
    expect(s.isAuthenticated).toBe(true)
    expect(s.needsMfa).toBe(true)
  })

  it('returns the API error code for a missing TOTP', async () => {
    fetchMock.mockRejectedValueOnce({ data: { error: 'totp_required' } })
    const s = useSessionStore()
    expect(await s.login('a@b.c', 'x')).toBe('totp_required')
    expect(s.isAuthenticated).toBe(false)
  })

  it('treats a failed /me as logged out', async () => {
    fetchMock.mockRejectedValueOnce({ statusCode: 401 })
    const s = useSessionStore()
    expect(await s.fetchMe()).toBeNull()
    expect(s.loaded).toBe(true)
  })

  it('clears the session on logout even if the request fails', async () => {
    const s = useSessionStore()
    s.me = { ...owner, roles: ['owner'] } as never
    fetchMock.mockRejectedValueOnce(new Error('offline'))
    await expect(s.logout()).rejects.toThrow()
    expect(s.me).toBeNull()
  })

  it('does not ask door or marketing staff for MFA', () => {
    const s = useSessionStore()
    s.me = { ...owner, roles: ['marketing'] } as never
    expect(s.needsMfa).toBe(false)
  })

  it('fetchMe stores the session and marks loaded', async () => {
    fetchMock.mockResolvedValue(owner)
    const s = useSessionStore()
    expect(await s.fetchMe()).toEqual(owner)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/auth/me', {})
    expect(s.isAuthenticated).toBe(true)
    expect(s.loaded).toBe(true)
  })

  it('login sends the TOTP when given', async () => {
    fetchMock.mockResolvedValueOnce({}).mockResolvedValueOnce(owner)
    const s = useSessionStore()
    expect(await s.login('a@b.c', 'pw', '123456')).toBeNull()
    expect(fetchMock.mock.calls[0]![1].body).toEqual({ email: 'a@b.c', password: 'pw', totp: '123456' })
  })

  it('login defaults the TOTP to an empty string', async () => {
    fetchMock.mockResolvedValueOnce({}).mockResolvedValueOnce(owner)
    const s = useSessionStore()
    await s.login('a@b.c', 'pw')
    expect(fetchMock.mock.calls[0]![1].body.totp).toBe('')
  })

  it('login falls back to network_error when the body has no code', async () => {
    fetchMock.mockRejectedValueOnce(new Error('offline'))
    const s = useSessionStore()
    expect(await s.login('a@b.c', 'pw')).toBe('network_error')
  })

  it('completeSetup POSTs token and password, returning null on success', async () => {
    fetchMock.mockResolvedValue({})
    const s = useSessionStore()
    expect(await s.completeSetup('tok', 'new password')).toBeNull()
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/auth/setup')
    expect(opts.method).toBe('POST')
    expect(opts.body).toEqual({ token: 'tok', password: 'new password' })
    expect(opts.headers['X-KlubHub-CSRF']).toBe('1')
  })

  it('completeSetup returns the API error code on failure', async () => {
    fetchMock.mockRejectedValue({ data: { error: 'invalid_token' } })
    const s = useSessionStore()
    expect(await s.completeSetup('tok', 'pw')).toBe('invalid_token')
  })

  it('enrollTotp returns the otpauth uri', async () => {
    fetchMock.mockResolvedValue({ otpauth_uri: 'otpauth://totp/x' })
    const s = useSessionStore()
    expect(await s.enrollTotp()).toBe('otpauth://totp/x')
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/auth/totp/enroll')
  })

  it('confirmTotp re-fetches the session on success', async () => {
    fetchMock.mockResolvedValueOnce({}).mockResolvedValueOnce({ ...owner, mfa: true })
    const s = useSessionStore()
    expect(await s.confirmTotp('123456')).toBeNull()
    expect(fetchMock.mock.calls[0]![1].body).toEqual({ code: '123456' })
    expect(s.me?.mfa).toBe(true)
  })

  it('confirmTotp returns the error code on failure', async () => {
    fetchMock.mockRejectedValue({ data: { error: 'totp_invalid' } })
    const s = useSessionStore()
    expect(await s.confirmTotp('000000')).toBe('totp_invalid')
  })

  it('gates management and editing hints by role', () => {
    const s = useSessionStore()
    expect(s.canManageOrg).toBe(false)
    expect(s.canEditEvents).toBe(false)
    s.me = { ...owner, roles: ['booker'] } as never
    expect(s.canManageOrg).toBe(false)
    expect(s.canEditEvents).toBe(true)
    expect(s.needsMfa).toBe(false)
    s.me = { ...owner, roles: ['finance'] } as never
    expect(s.canManageOrg).toBe(false)
    expect(s.canEditEvents).toBe(false)
    expect(s.needsMfa).toBe(true)
    s.me = { ...owner, roles: ['admin'], mfa: true } as never
    expect(s.canManageOrg).toBe(true)
    expect(s.needsMfa).toBe(false)
  })
})
