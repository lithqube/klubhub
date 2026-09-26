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
