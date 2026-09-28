import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAudienceStore } from '../audience'

const fetchMock = vi.fn()
vi.stubGlobal('$fetch', fetchMock)

const page = {
  contacts: [{ id: 'c1', name: 'Nadia', status: 'active', source: 'follow' }],
  counts: { all: 1, active: 1, unsubscribed: 0, bounced: 0, complained: 0 },
}

describe('useAudienceStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
  })

  it('loads contacts and segments together', async () => {
    fetchMock.mockImplementation((url: string) => Promise.resolve(url.endsWith('/segments') ? [{ id: 's1', name: 'RSVP' }] : page))
    const s = useAudienceStore()
    await s.load()
    expect(fetchMock.mock.calls.map(c => c[0]).sort()).toEqual(['/api/v1/audience/contacts', '/api/v1/audience/segments'])
    expect(s.contacts).toHaveLength(1)
    expect(s.segments).toHaveLength(1)
    expect(s.counts.active).toBe(1)
  })

  it('builds the query string from status, source and q', async () => {
    fetchMock.mockImplementation((url: string) => Promise.resolve(url.includes('/segments') ? [] : page))
    const s = useAudienceStore()
    await s.load({ status: 'active', source: 'csv', q: 'nadia' })
    const contactsUrl = fetchMock.mock.calls.map(c => c[0]).find((u: string) => u.startsWith('/api/v1/audience/contacts'))
    const params = new URLSearchParams(contactsUrl.split('?')[1])
    expect(params.get('status')).toBe('active')
    expect(params.get('source')).toBe('csv')
    expect(params.get('q')).toBe('nadia')
  })

  it('creates a contact and bumps counts by its own status, not a stale copy', async () => {
    fetchMock.mockResolvedValue({ id: 'c2', name: 'Lars', status: 'active', source: 'rsvp' })
    const s = useAudienceStore()
    const c = await s.createContact({ name: 'Lars', email: '', phone: '', source: 'rsvp', consent: { basis: 'consent', form_text: 'x' } })
    expect(c.id).toBe('c2')
    expect(s.contacts[0]!.id).toBe('c2')
    expect(s.counts.all).toBe(1)
    expect(s.counts.active).toBe(1)
    expect(s.counts.unsubscribed).toBe(0)
  })

  it('does not show a newly created contact when it does not match the active filter', async () => {
    fetchMock.mockImplementation((url: string) =>
      Promise.resolve(url.endsWith('/segments')
        ? []
        : { contacts: [], counts: { all: 0, active: 0, unsubscribed: 0, bounced: 0, complained: 0 } }))
    const s = useAudienceStore()
    // Viewing the UNSUBSCRIBED tab...
    await s.load({ status: 'unsubscribed' })
    fetchMock.mockResolvedValue({ id: 'c3', name: 'New', status: 'active', source: 'csv' })
    // ...adding a contact, which always defaults to active server-side.
    await s.createContact({ name: 'New', email: '', phone: '', source: 'csv', consent: { basis: 'soft_opt_in', form_text: 'x' } })
    // It must not appear in the unsubscribed-filtered list with the wrong
    // badge, even though the tenant-wide counts still include it.
    expect(s.contacts).toHaveLength(0)
    expect(s.counts.all).toBe(1)
    expect(s.counts.active).toBe(1)
  })

  it('shows a newly created contact when it matches the active filter', async () => {
    fetchMock.mockImplementation((url: string) => Promise.resolve(url.endsWith('/segments') ? [] : page))
    const s = useAudienceStore()
    await s.load({ status: 'active' })
    fetchMock.mockResolvedValue({ id: 'c3', name: 'New', status: 'active', source: 'csv' })
    await s.createContact({ name: 'New', email: '', phone: '', source: 'csv', consent: { basis: 'soft_opt_in', form_text: 'x' } })
    expect(s.contacts.map(c => c.id)).toContain('c3')
  })

  it('unsubscribing reloads the page rather than guessing the new counts', async () => {
    fetchMock.mockImplementation((url: string, opts?: { method?: string }) => {
      if (opts?.method === 'POST') return Promise.resolve(undefined)
      return Promise.resolve(url.endsWith('/segments') ? [] : { contacts: [], counts: { all: 1, active: 0, unsubscribed: 1, bounced: 0, complained: 0 } })
    })
    const s = useAudienceStore()
    await s.setStatus('c1', { status: 'unsubscribed' })
    expect(s.counts.unsubscribed).toBe(1)
  })

  it('deletes a contact locally without a full reload', async () => {
    fetchMock.mockResolvedValue(undefined)
    const s = useAudienceStore()
    s.contacts = [{ id: 'c1' } as never, { id: 'c2' } as never]
    await s.deleteContact('c1')
    expect(s.contacts.map(c => c.id)).toEqual(['c2'])
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/audience/contacts/c1')
    expect(opts.method).toBe('DELETE')
  })

  it('imports a CSV batch and reloads', async () => {
    fetchMock.mockImplementation((url: string, opts?: { method?: string }) => {
      if (opts?.method === 'POST' && url.endsWith('/import')) return Promise.resolve({ added: 2, updated: 0, duplicates: 1, invalid: 0 })
      return Promise.resolve(url.endsWith('/segments') ? [] : page)
    })
    const s = useAudienceStore()
    const r = await s.importCSV([{ name: 'A', email: 'a@example.com' }], { basis: 'soft_opt_in', form_text: 'past customers' })
    expect(r).toEqual({ added: 2, updated: 0, duplicates: 1, invalid: 0 })
    const importCall = fetchMock.mock.calls.find(c => (c[0] as string).endsWith('/import'))!
    expect((importCall[1] as { body: unknown }).body).toEqual({
      rows: [{ name: 'A', email: 'a@example.com' }],
      consent: { basis: 'soft_opt_in', form_text: 'past customers' },
    })
  })

  it('creates, updates and deletes a segment in place', async () => {
    const s = useAudienceStore()
    fetchMock.mockResolvedValue({ id: 'seg1', name: 'Berlin', filter: {}, matching: 0 })
    await s.createSegment({ name: 'Berlin', filter: {} })
    expect(s.segments).toEqual([{ id: 'seg1', name: 'Berlin', filter: {}, matching: 0 }])

    fetchMock.mockResolvedValue({ id: 'seg1', name: 'Berlin, techno', filter: {}, matching: 3 })
    await s.updateSegment('seg1', { name: 'Berlin, techno', filter: {} })
    expect(s.segments[0]!.matching).toBe(3)

    fetchMock.mockResolvedValue(undefined)
    await s.deleteSegment('seg1')
    expect(s.segments).toHaveLength(0)
  })

  it('builds an export URL that carries the active filter', () => {
    const s = useAudienceStore()
    expect(s.exportUrl({ status: 'bounced' })).toBe('/api/v1/audience/contacts/export.csv?status=bounced')
    expect(s.exportUrl()).toBe('/api/v1/audience/contacts/export.csv')
  })
})
