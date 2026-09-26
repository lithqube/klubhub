import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useRaStore } from '../ra'

const mockFetch = vi.fn()

function setup(raImport: boolean) {
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { features: { raImport } } }))
  vi.stubGlobal('$fetch', mockFetch)
  setActivePinia(createPinia())
  return useRaStore()
}

describe('useRaStore edition gating', () => {
  beforeEach(() => {
    mockFetch.mockReset()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('never calls the API when RA import is disabled', async () => {
    const store = setup(false)
    expect(store.enabled).toBe(false)

    await store.fetchArtist('test-artist')
    await store.fetchEvents('test-artist')
    await expect(store.importEvents({ artist_slug: 'test-artist' })).rejects.toThrow()
    await expect(store.importFromEpk({ artist_slug: 'test-artist' })).rejects.toThrow()

    expect(mockFetch).not.toHaveBeenCalled()
    expect(store.artist).toBeNull()
  })

  it('does not mention the integration in its disabled error', async () => {
    const store = setup(false)
    await expect(store.importEvents({ artist_slug: 'x' })).rejects.toThrow(/not available/)
    await expect(store.importEvents({ artist_slug: 'x' })).rejects.not.toThrow(/\bRA\b|Resident/)
  })

  it('calls the API when RA import is enabled', async () => {
    mockFetch.mockResolvedValue({ data: { id: '1', name: 'Test Artist', slug: 'test-artist' } })
    const store = setup(true)

    await store.fetchArtist('test-artist')

    expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/info/test-artist')
    expect(store.artist?.name).toBe('Test Artist')
  })
})
