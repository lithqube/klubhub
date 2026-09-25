import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useOrgStore } from '../org'

const fetchMock = vi.fn()
const EMPTY = { bio: '', website_url: null, instagram_url: null, soundcloud_url: null, ra_url: null, accent_color: null }
vi.stubGlobal('$fetch', fetchMock)

describe('useOrgStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
  })

  it('loads the organisation', async () => {
    fetchMock.mockResolvedValue({ id: 'o1', name: 'Nachtwerk', slug: 'nachtwerk', timezone: 'Europe/Berlin', currency: 'EUR' })
    const store = useOrgStore()
    await store.fetchOrg()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/org', {})
    expect(store.org?.name).toBe('Nachtwerk')
    expect(store.error).toBeNull()
    expect(store.loading).toBe(false)
  })

  it('surfaces a failure instead of keeping stale data', async () => {
    const store = useOrgStore()
    store.org = { id: 'old', name: 'Old', slug: 'old', timezone: 'UTC', currency: 'EUR', ...EMPTY }
    fetchMock.mockRejectedValue(new Error('offline'))
    await store.fetchOrg()
    expect(store.org).toBeNull()
    expect(store.error).toBe('offline')
    expect(store.loading).toBe(false)
  })

  it('saves the profile and keeps the returned organisation', async () => {
    const saved = { id: 'o1', name: 'Nachtwerk', slug: 'nachtwerk', timezone: 'Europe/Berlin', currency: 'EUR', ...EMPTY, bio: 'Techno' }
    fetchMock.mockResolvedValue(saved)
    const store = useOrgStore()
    await store.updateProfile({ ...EMPTY, bio: 'Techno' })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/org/profile', expect.objectContaining({ method: 'PUT' }))
    expect(store.org?.bio).toBe('Techno')
  })

  it('reports the invalid field', async () => {
    fetchMock.mockRejectedValue({ data: { error: 'invalid', field: 'website_url', problem: 'an https:// link' } })
    const store = useOrgStore()
    await expect(store.updateProfile(EMPTY)).rejects.toMatchObject({ error: 'invalid', field: 'website_url' })
  })
})
