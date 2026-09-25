import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useOrgStore } from '../org'

const fetchMock = vi.fn()
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
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/org')
    expect(store.org?.name).toBe('Nachtwerk')
    expect(store.error).toBeNull()
    expect(store.loading).toBe(false)
  })

  it('surfaces a failure instead of keeping stale data', async () => {
    const store = useOrgStore()
    store.org = { id: 'old', name: 'Old', slug: 'old', timezone: 'UTC', currency: 'EUR' }
    fetchMock.mockRejectedValue(new Error('offline'))
    await store.fetchOrg()
    expect(store.org).toBeNull()
    expect(store.error).toBe('offline')
    expect(store.loading).toBe(false)
  })
})
