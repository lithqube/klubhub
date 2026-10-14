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

describe('useRaStore enabled paths', () => {
  beforeEach(() => {
    mockFetch.mockReset()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('fetchArtist sets error from the API body and clears artist on failure', async () => {
    mockFetch.mockRejectedValue({ data: { error: 'artist not found' } })
    const store = setup(true)

    await store.fetchArtist('missing')

    expect(store.error).toBe('artist not found')
    expect(store.artist).toBeNull()
    expect(store.loading).toBe(false)
  })

  it('fetchArtist prefers body.message over the Error message', async () => {
    mockFetch.mockRejectedValue({ data: { message: 'rate limited' } })
    const store = setup(true)

    await store.fetchArtist('x')

    expect(store.error).toBe('rate limited')
  })

  it('fetchArtist falls back to the Error message, then a default', async () => {
    mockFetch.mockRejectedValue(new Error('network down'))
    const store = setup(true)
    await store.fetchArtist('x')
    expect(store.error).toBe('network down')

    mockFetch.mockRejectedValue('string failure')
    await store.fetchArtist('x')
    expect(store.error).toBe('Failed to fetch artist')
  })

  it('fetchEvents loads the artist through fetchArtist', async () => {
    mockFetch.mockResolvedValue({ data: { id: '1', name: 'A', slug: 'a' } })
    const store = setup(true)

    await store.fetchEvents('a')

    expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/info/a')
    expect(store.artist?.slug).toBe('a')
    expect(store.loading).toBe(false)
  })

  it('importEvents POSTs the request and stores the result', async () => {
    const result = { imported: 3, skipped: 1 }
    mockFetch.mockResolvedValue(result)
    const store = setup(true)

    const returned = await store.importEvents({ artist_slug: 'a' })

    expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/import-ra', {
      method: 'POST',
      body: { artist_slug: 'a' },
    })
    expect(returned).toEqual(result)
    expect(store.importResult).toEqual(result)
    expect(store.loading).toBe(false)
  })

  it('importEvents records the API error and rethrows', async () => {
    mockFetch.mockRejectedValue({ data: { error: 'import exploded' } })
    const store = setup(true)

    await expect(store.importEvents({ artist_slug: 'a' })).rejects.toEqual({
      data: { error: 'import exploded' },
    })
    expect(store.error).toBe('import exploded')
    expect(store.loading).toBe(false)
  })

  it('importFromEpk POSTs to the EPK route and clears error on success', async () => {
    const result = { imported: 1, skipped: 0 }
    mockFetch.mockResolvedValue(result)
    const store = setup(true)
    store.error = 'stale'

    const returned = await store.importFromEpk({ artist_slug: 'a' })

    expect(mockFetch).toHaveBeenCalledWith('/api/v1/epk/import-ra', {
      method: 'POST',
      body: { artist_slug: 'a' },
    })
    expect(returned).toEqual(result)
    expect(store.error).toBeNull()
  })

  it('importFromEpk records the error and rethrows', async () => {
    mockFetch.mockRejectedValue(new Error('epk down'))
    const store = setup(true)

    await expect(store.importFromEpk({ artist_slug: 'a' })).rejects.toThrow('epk down')
    expect(store.error).toBe('epk down')
  })

  it('clear() resets artist, events, importResult and error', async () => {
    mockFetch.mockResolvedValue({ data: { id: '1', name: 'A', slug: 'a' } })
    const store = setup(true)
    await store.fetchArtist('a')
    store.events = [{ id: 'e1' } as never]
    store.importResult = { imported: 1, skipped: 0 }
    store.error = 'x'

    store.clear()

    expect(store.artist).toBeNull()
    expect(store.events).toEqual([])
    expect(store.importResult).toBeNull()
    expect(store.error).toBeNull()
  })
})
