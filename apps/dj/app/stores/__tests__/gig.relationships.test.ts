import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useGigStore } from '../gig'
import type { Gig } from '../../types/gig'

const timestamps = { created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z', deleted_at: null }
const gig: Gig = {
  id: 'gig-1', date: '2026-10-01', venue: 'Legacy venue', city: 'Berlin', country: 'DE',
  event_name: 'Event', promoter_name: 'Legacy promoter', promoter_email: '', promoter_phone: '',
  fee_amount: 500, fee_currency: 'EUR', set_length_minutes: 90, notes: '', status: 'confirmed',
  payment_status: 'unpaid', gig_reader_venue_id: null, gig_reader_contact_id: null, ...timestamps,
  linkedVenues: [{ id: 'venue-1', name: 'Reusable venue', city: 'Berlin', country: 'DE',
    capacity: null, website: null, tech_contact_name: '', tech_contact_email: 'tech@example.com',
    tech_contact_phone: '', notes: '', ...timestamps }],
  linkedContacts: [{ id: 'contact-1', name: 'Reusable contact', company: null,
    email: 'promoter@example.com', phone: '', type: 'promoter', notes: '', ...timestamps }],
  linkedTracklists: [{ id: 'tracklist-1', title: 'Set', sourceFormat: 'rekordbox', rawFilePath: '',
    preset: 'story', visibleFields: ['title'], bgMode: 'solid', bgValue: '#000000', maxTracks: 20,
    trackRangeStart: 1, trackRangeEnd: 20, createdAt: timestamps.created_at, updatedAt: timestamps.updated_at }],
}

describe('gig relationship payload compatibility', () => {
  beforeEach(() => setActivePinia(createPinia()))
  afterEach(() => vi.unstubAllGlobals())

  it.each(['populated', 'empty', 'null'] as const)('preserves %s camelCase relationships in enveloped lists', async (state) => {
    const payload: Gig = state === 'populated' ? gig : {
      ...gig,
      linkedVenues: state === 'empty' ? [] : null,
      linkedContacts: state === 'empty' ? [] : null,
      linkedTracklists: state === 'empty' ? [] : null,
    }
    vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({ data: [payload] }))
    const store = useGigStore()
    await store.fetchGigs()
    expect(store.gigs).toEqual([payload])
    expect(store.gigs[0]?.venue).toBe('Legacy venue')
    expect(store.gigs[0]?.promoter_name).toBe('Legacy promoter')
    expect(store.gigs[0]?.linkedVenues).toEqual(payload.linkedVenues)
    expect(store.gigs[0]?.linkedContacts).toEqual(payload.linkedContacts)
    expect(store.gigs[0]?.linkedTracklists).toEqual(payload.linkedTracklists)
  })

  it('keeps the legacy detail tracklists separate from linkedTracklists', async () => {
    const detail = { ...gig, tracklists: [{ id: 'tracklist-1', title: 'Set' }] }
    const fetch = vi.fn().mockResolvedValue({ data: detail })
    vi.stubGlobal('$fetch', fetch)
    const store = useGigStore()
    expect(await store.fetchGigDetail(gig.id)).toEqual(detail)
    expect(fetch).toHaveBeenCalledWith('/api/v1/gigs/gig-1/detail')
  })
})
