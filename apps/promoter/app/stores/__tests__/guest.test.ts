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
})
