import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useGuestStore } from '../guest'

const fetchMock = vi.fn()
vi.stubGlobal('$fetch', fetchMock)

const page = { guests: [{ id: 'g1' }], counts: { all: 1, going: 1, pending: 0, waitlist: 0, invited: 0, declined: 0, going_heads: 1 } }

describe('useGuestStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
  })

  it('loads lists and the guest table of an event together', async () => {
    fetchMock.mockImplementation((url: string) => Promise.resolve(url.endsWith('/lists') ? [{ id: 'l1' }] : page))
    const s = useGuestStore()
    await s.load('e1')
    expect(fetchMock.mock.calls.map(c => c[0]).sort()).toEqual(['/api/v1/events/e1/guests', '/api/v1/events/e1/lists'])
    expect(s.lists).toHaveLength(1)
    expect(s.guests).toHaveLength(1)
    expect(s.counts.going_heads).toBe(1)
  })

  it('adds guests with CSRF and refreshes lists and guests', async () => {
    fetchMock.mockImplementation((url: string, opts?: { method?: string }) => {
      if (opts?.method === 'POST') return Promise.resolve({ added: [{ id: 'g2' }], duplicates: [] })
      return Promise.resolve(url.endsWith('/lists') ? [] : page)
    })
    const s = useGuestStore()
    s.eventId = 'e1'
    const r = await s.addGuests({ list_id: 'l1', allocation_id: null, source: 'paste', guests: [{ name: 'Mara', plus_n: 1 }] })
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/events/e1/guests')
    expect(opts.headers['X-KlubHub-CSRF']).toBe('1')
    expect(opts.body.guests[0]).toEqual({ name: 'Mara', plus_n: 1 })
    expect(r.added).toHaveLength(1)
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })

  it('keeps the quota details of a refusal', async () => {
    fetchMock.mockRejectedValue({ statusCode: 409, data: { error: 'quota_exceeded', label: 'Ben Klock', quota: 6, used: 5, requested: 2 } })
    const s = useGuestStore()
    s.eventId = 'e1'
    await expect(s.addGuests({ list_id: 'l1', allocation_id: 'a1', source: 'manual', guests: [{ name: 'X', plus_n: 1 }] }))
      .rejects.toMatchObject({ error: 'quota_exceeded', status: 409, detail: { label: 'Ben Klock', used: 5 } })
  })

  it('sends bulk status by email and forces list deletion only when asked', async () => {
    const lists = [{ id: 'l1' }, { id: 'l2' }]
    fetchMock.mockImplementation((url: string) =>
      Promise.resolve(url.endsWith('bulk-status') ? { matched: 1, updated: 1, unmatched: [] } : url.endsWith('/lists') ? lists : page))
    const s = useGuestStore()
    s.eventId = 'e1'
    const r = await s.bulkStatus({ status: 'going', emails: ['a@example.org'] })
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/events/e1/guests/bulk-status')
    expect(r.updated).toBe(1)
    expect(s.lists).toHaveLength(2)
    fetchMock.mockReset()
    fetchMock.mockImplementation((url: string) => Promise.resolve(url.endsWith('/guests') ? page : null))
    await s.deleteList('l1')
    expect(fetchMock.mock.calls[0]![1].query).toBeUndefined()
    await s.deleteList('l2', true)
    expect(fetchMock.mock.calls[1]![1].query).toEqual({ force: 'true' })
    expect(s.lists).toHaveLength(0)
  })

  it('exports CSV as text with the list filter', async () => {
    fetchMock.mockResolvedValue('name\n')
    const s = useGuestStore()
    s.eventId = 'e1'
    expect(await s.exportCsv({ list_id: 'l1' })).toBe('name\n')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/events/e1/guests/export.csv', { query: { list_id: 'l1' }, responseType: 'text' })
  })

  it('upserts standing lists in place', async () => {
    const s = useGuestStore()
    fetchMock.mockResolvedValue({ id: 's1', name: 'Residents' })
    await s.saveStanding({} as never)
    fetchMock.mockResolvedValue({ id: 's1', name: 'Residents & friends' })
    await s.saveStanding({} as never, 's1')
    expect(fetchMock.mock.calls[1]![0]).toBe('/api/v1/standing-lists/s1')
    expect(s.standing).toEqual([{ id: 's1', name: 'Residents & friends' }])
  })

  it('uploads an export as multipart: dry run by default, a real run refreshes tickets', async () => {
    const ticket = { id: 'p1', name: 'Lena Vogt', status: 'valid' }
    fetchMock.mockImplementation((url: string, opts?: { method?: string }) => {
      if (opts?.method === 'POST') return Promise.resolve({ dry_run: true, counts: { positions_new: 1 } })
      return Promise.resolve({ ...page, tickets: [ticket] })
    })
    const s = useGuestStore()
    s.eventId = 'e1'
    const file = new Blob(['Order ID,Name\n1,Lena\n'], { type: 'text/csv' })
    await s.importAttendees({ preset: 'generic', file, fileName: 'ra.csv', mapping: { name: 'Name', order_ref: 'Order ID' } })
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/events/e1/attendees/import')
    expect(opts.query).toEqual({ dry_run: 'true' })
    expect(opts.headers['X-KlubHub-CSRF']).toBe('1')
    const form = opts.body as FormData
    expect(form.get('preset')).toBe('generic')
    expect(JSON.parse(form.get('mapping') as string)).toEqual({ name: 'Name', order_ref: 'Order ID' })
    expect((form.get('file') as File).name).toBe('ra.csv')
    expect(fetchMock).toHaveBeenCalledTimes(1) // a dry run changes nothing to refresh

    await s.importAttendees({ preset: 'dice', file, mapping: { name: 'x' } }, false)
    const [, real] = fetchMock.mock.calls[1]!
    expect(real.query).toEqual({ dry_run: 'false' })
    expect((real.body as FormData).get('mapping')).toBeNull() // only generic sends a mapping
    expect(s.tickets).toEqual([ticket])
  })

  it('defaults the import file name to export.csv', async () => {
    fetchMock.mockResolvedValue({ dry_run: true, counts: {} })
    const s = useGuestStore()
    s.eventId = 'e1'
    await s.importAttendees({ preset: 'dice', file: new Blob(['x']) })
    const form = fetchMock.mock.calls[0]![1].body as FormData
    expect((form.get('file') as File).name).toBe('export.csv')
  })

  it('records an ApiError when load fails', async () => {
    fetchMock.mockRejectedValue({ statusCode: 403, data: { error: 'forbidden' } })
    const s = useGuestStore()
    await s.load('e1')
    expect(s.error).toMatchObject({ error: 'forbidden', status: 403 })
    expect(s.loading).toBe(false)
    expect(s.eventId).toBe('e1')
  })

  it('refreshGuests is a no-op without an event', async () => {
    const s = useGuestStore()
    await s.refreshGuests()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('refreshLists is a no-op without an event', async () => {
    const s = useGuestStore()
    await s.refreshLists()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('refreshGuests applies the filter query and tolerates missing tickets', async () => {
    fetchMock.mockResolvedValue({ guests: [], counts: { all: 0 } })
    const s = useGuestStore()
    s.eventId = 'e1'
    s.tickets = [{ id: 'stale' } as never]
    await s.refreshGuests({ status: 'going' })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/events/e1/guests', { query: { status: 'going' } })
    expect(s.tickets).toEqual([])
  })

  it('creates a list and appends it', async () => {
    fetchMock.mockResolvedValue({ id: 'l9', name: 'Press' })
    const s = useGuestStore()
    s.eventId = 'e1'
    const l = await s.createList({ name: 'Press' } as never)
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/events/e1/lists')
    expect(opts.method).toBe('POST')
    expect(opts.headers['X-KlubHub-CSRF']).toBe('1')
    expect(s.lists).toEqual([l])
  })

  it('updates a list in place and refreshes guests', async () => {
    fetchMock.mockImplementation((url: string, opts?: { method?: string }) =>
      Promise.resolve(opts?.method === 'PUT' ? { id: 'l1', name: 'VIP' } : page))
    const s = useGuestStore()
    s.eventId = 'e1'
    s.lists = [{ id: 'l1', name: 'Guest' } as never, { id: 'l2', name: 'Press' } as never]
    const l = await s.updateList('l1', { name: 'VIP' } as never)
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/events/e1/lists/l1')
    expect(l.name).toBe('VIP')
    expect(s.lists.map(x => x.name)).toEqual(['VIP', 'Press'])
    expect(fetchMock).toHaveBeenCalledTimes(2) // PUT + refreshGuests
  })

  it('manages allocations with CSRF and refreshes lists', async () => {
    fetchMock.mockImplementation((url: string, opts?: { method?: string }) =>
      Promise.resolve(opts?.method ? { id: 'a1', quota: 10 } : [{ id: 'l1' }]))
    const s = useGuestStore()
    s.eventId = 'e1'
    await s.createAllocation('l1', { quota: 10 } as never)
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/events/e1/lists/l1/allocations')
    expect(fetchMock.mock.calls[0]![1].method).toBe('POST')
    await s.updateAllocation('l1', 'a1', { quota: 20 } as never)
    expect(fetchMock.mock.calls[2]![0]).toBe('/api/v1/events/e1/lists/l1/allocations/a1')
    expect(fetchMock.mock.calls[2]![1].method).toBe('PUT')
    await s.revokeAllocation('l1', 'a1')
    expect(fetchMock.mock.calls[4]![0]).toBe('/api/v1/events/e1/lists/l1/allocations/a1')
    expect(fetchMock.mock.calls[4]![1].method).toBe('DELETE')
    for (const call of fetchMock.mock.calls) {
      if (call[1]?.method) expect(call[1].headers['X-KlubHub-CSRF']).toBe('1')
    }
  })

  it('updates and deletes a guest, refreshing both views', async () => {
    fetchMock.mockImplementation((url: string, opts?: { method?: string }) =>
      Promise.resolve(opts?.method === 'PUT' ? { id: 'g1', name: 'Renamed' } : url.endsWith('/lists') ? [] : page))
    const s = useGuestStore()
    s.eventId = 'e1'
    const g = await s.updateGuest('g1', { name: 'Renamed' } as never)
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/events/e1/guests/g1')
    expect(g.name).toBe('Renamed')
    fetchMock.mockClear()
    fetchMock.mockImplementation((url: string) => Promise.resolve(url.endsWith('/lists') ? [] : page))
    await s.deleteGuest('g1')
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/events/e1/guests/g1')
    expect(fetchMock.mock.calls[0]![1].method).toBe('DELETE')
    expect(fetchMock).toHaveBeenCalledTimes(3) // DELETE + refresh (guests + lists)
  })

  it('fetches standing lists and deletes one', async () => {
    fetchMock.mockResolvedValue([{ id: 's1' }, { id: 's2' }])
    const s = useGuestStore()
    await s.fetchStanding()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/standing-lists', {})
    expect(s.standing).toHaveLength(2)
    fetchMock.mockResolvedValue(undefined)
    await s.deleteStanding('s1')
    expect(fetchMock.mock.calls[1]![0]).toBe('/api/v1/standing-lists/s1')
    expect(fetchMock.mock.calls[1]![1].method).toBe('DELETE')
    expect(s.standing.map(x => x.id)).toEqual(['s2'])
  })

  it('fetchOverview loads rows and records failures', async () => {
    fetchMock.mockResolvedValue([{ event_id: 'e1', going: 3 }])
    const s = useGuestStore()
    await s.fetchOverview()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/guests/overview', {})
    expect(s.overview).toEqual([{ event_id: 'e1', going: 3 }])
    expect(s.loading).toBe(false)

    fetchMock.mockRejectedValue({ statusCode: 500, data: { error: 'boom' } })
    await s.fetchOverview()
    expect(s.error).toMatchObject({ error: 'boom', status: 500 })
    expect(s.loading).toBe(false)
  })

  it('wraps action failures in an ApiError', async () => {
    fetchMock.mockRejectedValue({ statusCode: 422, data: { error: 'invalid', field: 'name' } })
    const s = useGuestStore()
    s.eventId = 'e1'
    await expect(s.createList({} as never)).rejects.toMatchObject({ error: 'invalid', field: 'name', status: 422 })
  })
})
