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
})
