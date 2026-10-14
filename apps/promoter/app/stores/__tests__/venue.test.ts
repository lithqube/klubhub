import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useVenueStore } from '../venue'

const fetchMock = vi.fn()
vi.stubGlobal('$fetch', fetchMock)

const venue = { id: 'v1', name: 'Berghain' }

describe('useVenueStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
  })

  describe('fetchVenues', () => {
    it('loads the list and clears loading', async () => {
      fetchMock.mockResolvedValue([venue])
      const s = useVenueStore()
      await s.fetchVenues()
      expect(fetchMock).toHaveBeenCalledWith('/api/v1/venues', {})
      expect(s.venues).toEqual([venue])
      expect(s.loading).toBe(false)
      expect(s.error).toBeNull()
    })

    it('records an ApiError and stops loading on failure', async () => {
      fetchMock.mockRejectedValue({ statusCode: 500, data: { error: 'boom' } })
      const s = useVenueStore()
      await s.fetchVenues()
      expect(s.venues).toEqual([])
      expect(s.loading).toBe(false)
      expect(s.error).toMatchObject({ error: 'boom', status: 500 })
    })
  })

  describe('getVenue', () => {
    it('returns the venue', async () => {
      fetchMock.mockResolvedValue(venue)
      const s = useVenueStore()
      expect(await s.getVenue('v1')).toEqual(venue)
      expect(fetchMock).toHaveBeenCalledWith('/api/v1/venues/v1', expect.anything())
    })

    it('propagates the raw error', async () => {
      fetchMock.mockRejectedValue(new Error('offline'))
      const s = useVenueStore()
      await expect(s.getVenue('v1')).rejects.toThrow('offline')
    })
  })

  describe('saveVenue', () => {
    it('creates with POST and inserts sorted by name', async () => {
      fetchMock.mockResolvedValue({ id: 'a', name: 'Berghain' })
      const s = useVenueStore()
      s.venues = [{ id: 'b', name: 'Zoo' } as never]
      const saved = await s.saveVenue({ name: 'Berghain' } as never)
      const [url, opts] = fetchMock.mock.calls[0]!
      expect(url).toBe('/api/v1/venues')
      expect(opts.method).toBe('POST')
      expect(opts.headers['X-KlubHub-CSRF']).toBe('1')
      expect(saved.id).toBe('a')
      expect(s.venues.map(v => v.name)).toEqual(['Berghain', 'Zoo'])
    })

    it('updates with PUT and replaces in place', async () => {
      fetchMock.mockResolvedValue({ id: 'v1', name: 'Renamed' })
      const s = useVenueStore()
      s.venues = [{ id: 'v1', name: 'Old' } as never, { id: 'v2', name: 'Other' } as never]
      const saved = await s.saveVenue({ name: 'Renamed' } as never, 'v1')
      const [url, opts] = fetchMock.mock.calls[0]!
      expect(url).toBe('/api/v1/venues/v1')
      expect(opts.method).toBe('PUT')
      expect(saved.name).toBe('Renamed')
      expect(s.venues.map(v => v.name)).toEqual(['Renamed', 'Other'])
    })

    it('wraps failures in an ApiError', async () => {
      fetchMock.mockRejectedValue({ statusCode: 422, data: { error: 'invalid', field: 'name' } })
      const s = useVenueStore()
      await expect(s.saveVenue({} as never)).rejects.toMatchObject({ error: 'invalid', field: 'name', status: 422 })
    })
  })

  describe('archiveVenue', () => {
    it('DELETEs with CSRF and removes the venue', async () => {
      fetchMock.mockResolvedValue(undefined)
      const s = useVenueStore()
      s.venues = [venue as never, { id: 'v2', name: 'Other' } as never]
      await s.archiveVenue('v1')
      const [url, opts] = fetchMock.mock.calls[0]!
      expect(url).toBe('/api/v1/venues/v1')
      expect(opts.method).toBe('DELETE')
      expect(opts.headers['X-KlubHub-CSRF']).toBe('1')
      expect(s.venues.map(v => v.id)).toEqual(['v2'])
    })

    it('keeps the list and wraps the failure', async () => {
      fetchMock.mockRejectedValue({ statusCode: 404, data: { error: 'not_found' } })
      const s = useVenueStore()
      s.venues = [venue as never]
      await expect(s.archiveVenue('v1')).rejects.toMatchObject({ error: 'not_found', status: 404 })
      expect(s.venues).toHaveLength(1)
    })
  })

  describe('revealVenue', () => {
    it('POSTs to the reveal route and returns the protected venue', async () => {
      const protectedVenue = { id: 'v1', name: 'Berghain', tech_notes: 'secret' }
      fetchMock.mockResolvedValue(protectedVenue)
      const s = useVenueStore()
      expect(await s.revealVenue('v1')).toEqual(protectedVenue)
      const [url, opts] = fetchMock.mock.calls[0]!
      expect(url).toBe('/api/v1/venues/v1/reveal')
      expect(opts.method).toBe('POST')
      expect(opts.headers['X-KlubHub-CSRF']).toBe('1')
    })
  })
})
