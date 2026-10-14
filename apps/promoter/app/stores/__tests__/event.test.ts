import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useEventStore } from '../event'
import { useVenueStore } from '../venue'

const fetchMock = vi.fn()
vi.stubGlobal('$fetch', fetchMock)

describe('useEventStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
  })

  it('loads a list view with the view query', async () => {
    fetchMock.mockResolvedValue([{ id: 'e1' }])
    const s = useEventStore()
    await s.fetchList('drafts')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/events', { query: { view: 'drafts' } })
    expect(s.lists.drafts).toHaveLength(1)
  })

  it('sends the version with lineup changes and keeps the returned detail', async () => {
    fetchMock.mockResolvedValue({ id: 'e1', version: 4, issues: [] })
    const s = useEventStore()
    await s.replaceLineup('e1', 3, [])
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/events/e1/lineup')
    expect(opts.method).toBe('PUT')
    expect(opts.body).toEqual({ version: 3, lineup: [] })
    expect(opts.headers['X-KlubHub-CSRF']).toBe('1')
    expect(s.current?.version).toBe(4)
  })

  it('surfaces blocked publishing with its issues', async () => {
    fetchMock.mockRejectedValue({ statusCode: 409, data: { error: 'timetable_errors', issues: [{ code: 'overlap', severity: 'error' }] } })
    const s = useEventStore()
    await expect(s.changeStatus('e1', 2, 'published')).rejects.toMatchObject({ error: 'timetable_errors', status: 409, issues: [{ code: 'overlap' }] })
  })

  it('maps validation errors to a field', async () => {
    fetchMock.mockRejectedValue({ statusCode: 422, data: { error: 'invalid', field: 'title', problem: 'required' } })
    await expect(useEventStore().createEvent({} as never)).rejects.toMatchObject({ field: 'title', problem: 'required' })
  })
})

describe('useVenueStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
  })

  it('inserts a created venue in name order', async () => {
    const s = useVenueStore()
    s.venues = [{ id: 'b', name: 'Zoo' } as never]
    fetchMock.mockResolvedValue({ id: 'a', name: 'Berghain' })
    await s.saveVenue({} as never)
    expect(s.venues.map(v => v.name)).toEqual(['Berghain', 'Zoo'])
  })

  it('reports an MFA refusal on reveal', async () => {
    fetchMock.mockRejectedValue({ statusCode: 403, data: { error: 'mfa_required' } })
    await expect(useVenueStore().revealVenue('v1')).rejects.toMatchObject({ error: 'mfa_required', status: 403 })
  })
})

describe('useEventStore detail and exports', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
  })

  it('fetchList records an ApiError and stops loading on failure', async () => {
    fetchMock.mockRejectedValue({ statusCode: 500, data: { error: 'boom' } })
    const s = useEventStore()
    await s.fetchList('upcoming')
    expect(s.error).toMatchObject({ error: 'boom', status: 500 })
    expect(s.loading).toBe(false)
    expect(s.lists.upcoming).toEqual([])
  })

  it('fetchEvent stores the detail', async () => {
    fetchMock.mockResolvedValue({ id: 'e1', version: 1 })
    const s = useEventStore()
    expect(await s.fetchEvent('e1')).toEqual({ id: 'e1', version: 1 })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/events/e1', {})
    expect(s.current?.id).toBe('e1')
    expect(s.error).toBeNull()
  })

  it('fetchEvent clears current and records the error on failure', async () => {
    fetchMock.mockRejectedValue({ statusCode: 404, data: { error: 'not_found' } })
    const s = useEventStore()
    s.current = { id: 'stale' } as never
    expect(await s.fetchEvent('e1')).toBeNull()
    expect(s.current).toBeNull()
    expect(s.error).toMatchObject({ error: 'not_found', status: 404 })
  })

  it('updateEvent PUTs version and event and keeps the detail', async () => {
    fetchMock.mockResolvedValue({ id: 'e1', version: 2 })
    const s = useEventStore()
    const input = { title: 'Party' } as never
    const detail = await s.updateEvent('e1', 1, input)
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/events/e1')
    expect(opts.method).toBe('PUT')
    expect(opts.body).toEqual({ version: 1, event: input })
    expect(opts.headers['X-KlubHub-CSRF']).toBe('1')
    expect(detail.version).toBe(2)
    expect(s.current?.version).toBe(2)
  })

  it('replaceStages PUTs the version and stages', async () => {
    fetchMock.mockResolvedValue({ id: 'e1', version: 5 })
    const s = useEventStore()
    await s.replaceStages('e1', 4, [{ name: 'Main' }] as never)
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/events/e1/stages')
    expect(opts.method).toBe('PUT')
    expect(opts.body).toEqual({ version: 4, stages: [{ name: 'Main' }] })
  })

  it('changeStatus POSTs version and status', async () => {
    fetchMock.mockResolvedValue({ id: 'e1', version: 3, status: 'published' })
    const s = useEventStore()
    await s.changeStatus('e1', 2, 'published')
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/events/e1/status')
    expect(opts.method).toBe('POST')
    expect(opts.body).toEqual({ version: 2, status: 'published' })
  })

  it('fetches the export pack, JSON-LD and ICS', async () => {
    fetchMock.mockResolvedValueOnce({ events: [] })
      .mockResolvedValueOnce({ '@context': 'https://schema.org' })
      .mockResolvedValueOnce('BEGIN:VCALENDAR')
    const s = useEventStore()
    expect(await s.fetchExport('e1')).toEqual({ events: [] })
    expect(await s.fetchJsonLd('e1')).toEqual({ '@context': 'https://schema.org' })
    expect(await s.fetchIcs('e1')).toBe('BEGIN:VCALENDAR')
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/events/e1/export')
    expect(fetchMock.mock.calls[1]![0]).toBe('/api/v1/events/e1/export/jsonld')
    expect(fetchMock.mock.calls[2]![0]).toBe('/api/v1/events/e1/export/ics')
    expect(fetchMock.mock.calls[2]![1].responseType).toBe('text')
  })

  it('wraps export failures in an ApiError', async () => {
    fetchMock.mockRejectedValue({ statusCode: 503, data: { error: 'unavailable' } })
    const s = useEventStore()
    await expect(s.fetchExport('e1')).rejects.toMatchObject({ error: 'unavailable', status: 503 })
    await expect(s.fetchJsonLd('e1')).rejects.toMatchObject({ error: 'unavailable', status: 503 })
    await expect(s.fetchIcs('e1')).rejects.toMatchObject({ error: 'unavailable', status: 503 })
  })
})
