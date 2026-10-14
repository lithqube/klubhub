import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useGigStore } from '../gig';
import type { Gig, GigCreate } from '../../types/gig';

function makeGig(overrides: Partial<Gig> = {}): Gig {
  return {
    id: 'gig-1',
    date: '2026-11-01',
    venue: 'Berghain',
    city: 'Berlin',
    country: 'DE',
    event_name: 'Klubnacht',
    promoter_name: 'Promoter',
    promoter_email: 'p@example.com',
    promoter_phone: '+49 000',
    fee_amount: 1500,
    fee_currency: 'EUR',
    set_length_minutes: 120,
    notes: '',
    status: 'confirmed',
    payment_status: 'unpaid',
    gig_reader_venue_id: null,
    gig_reader_contact_id: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    deleted_at: null,
    ...overrides,
  };
}

const mockFetch = vi.fn();

function setup(icalSecret = 'test-secret') {
  vi.stubGlobal('$fetch', mockFetch);
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { icalSecret } }));
  setActivePinia(createPinia());
  return useGigStore();
}

describe('useGigStore', () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe('initial state', () => {
    it('starts empty with default filters', () => {
      const store = setup();
      expect(store.gigs).toEqual([]);
      expect(store.loading).toBe(false);
      expect(store.filters).toEqual({
        status: '',
        venue: '',
        city: '',
        fee_min: null,
        fee_max: null,
        from: '',
        to: '',
      });
    });
  });

  describe('computed getters', () => {
    it('upcomingGigs excludes played and cancelled', () => {
      const store = setup();
      store.gigs = [
        makeGig({ id: 'a', status: 'played' }),
        makeGig({ id: 'b', status: 'cancelled' }),
        makeGig({ id: 'c', status: 'confirmed' }),
        makeGig({ id: 'd', status: 'inquiry' }),
      ];
      expect(store.upcomingGigs.map((g) => g.id)).toEqual(['c', 'd']);
    });

    it('ytdEarned sums fees of played gigs in the current year only', () => {
      const store = setup();
      const year = new Date().getFullYear();
      store.gigs = [
        makeGig({ id: 'a', status: 'played', date: `${year}-03-01`, fee_amount: 1000 }),
        makeGig({ id: 'b', status: 'played', date: `${year}-06-01`, fee_amount: '500' as unknown as number }),
        makeGig({ id: 'c', status: 'played', date: `${year - 1}-06-01`, fee_amount: 9999 }),
        makeGig({ id: 'd', status: 'confirmed', date: `${year}-06-01`, fee_amount: 9999 }),
      ];
      expect(store.ytdEarned).toBe(1500);
    });

    it('ytdEarned is 0 when no played gigs exist', () => {
      const store = setup();
      expect(store.ytdEarned).toBe(0);
    });

    it('avgFee averages fees of played gigs, rounded', () => {
      const store = setup();
      store.gigs = [
        makeGig({ id: 'a', status: 'played', fee_amount: 1000 }),
        makeGig({ id: 'b', status: 'played', fee_amount: 500 }),
        makeGig({ id: 'c', status: 'played', fee_amount: 501 }),
        makeGig({ id: 'd', status: 'confirmed', fee_amount: 9999 }),
      ];
      expect(store.avgFee).toBe(Math.round(2001 / 3));
    });

    it('avgFee is 0 when no played gigs exist', () => {
      const store = setup();
      expect(store.avgFee).toBe(0);
    });
  });

  describe('fetchGigs', () => {
    it('fetches without query string when filters are empty', async () => {
      mockFetch.mockResolvedValue([makeGig()]);
      const store = setup();
      await store.fetchGigs();
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs');
      expect(store.gigs).toHaveLength(1);
      expect(store.loading).toBe(false);
    });

    it('serializes non-empty filters into the query string', async () => {
      mockFetch.mockResolvedValue([]);
      const store = setup();
      store.setFilter('status', 'confirmed');
      store.setFilter('city', 'Berlin');
      store.setFilter('fee_min', 100);
      await store.fetchGigs();
      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain('status=confirmed');
      expect(url).toContain('city=Berlin');
      expect(url).toContain('fee_min=100');
      // empty-string and null filters are omitted
      expect(url).not.toContain('venue=');
      expect(url).not.toContain('fee_max=');
    });

    it('unwraps a { data } envelope', async () => {
      mockFetch.mockResolvedValue({ data: [makeGig()] });
      const store = setup();
      await store.fetchGigs();
      expect(store.gigs).toHaveLength(1);
    });

    it('treats a JSON null response as an empty list', async () => {
      mockFetch.mockResolvedValue(null);
      const store = setup();
      await store.fetchGigs();
      expect(store.gigs).toEqual([]);
    });

    it('clears gigs and stops loading on error', async () => {
      mockFetch.mockRejectedValue(new Error('boom'));
      const store = setup();
      store.gigs = [makeGig()];
      const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
      await store.fetchGigs();
      expect(store.gigs).toEqual([]);
      expect(store.loading).toBe(false);
      spy.mockRestore();
    });
  });

  describe('fetchGigDetail', () => {
    it('returns the bare resource', async () => {
      mockFetch.mockResolvedValue(makeGig({ id: 'gig-9' }));
      const store = setup();
      const detail = await store.fetchGigDetail('gig-9');
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/gig-9/detail');
      expect(detail?.id).toBe('gig-9');
    });

    it('unwraps a { data } envelope', async () => {
      mockFetch.mockResolvedValue({ data: makeGig({ id: 'gig-9' }) });
      const store = setup();
      const detail = await store.fetchGigDetail('gig-9');
      expect(detail?.id).toBe('gig-9');
    });

    it('returns null on error', async () => {
      mockFetch.mockRejectedValue(new Error('boom'));
      const store = setup();
      const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
      expect(await store.fetchGigDetail('gig-9')).toBeNull();
      spy.mockRestore();
    });
  });

  describe('createGig', () => {
    const payload: GigCreate = {
      date: '2026-12-01',
      venue: 'Venue',
      city: 'City',
      country: 'DE',
      event_name: 'Event',
      promoter_name: 'P',
      promoter_email: 'p@example.com',
      promoter_phone: '',
      fee_amount: 100,
      fee_currency: 'EUR',
    };

    it('POSTs and unshifts the created gig', async () => {
      const created = makeGig({ id: 'new' });
      mockFetch.mockResolvedValue(created);
      const store = setup();
      store.gigs = [makeGig({ id: 'old' })];
      const result = await store.createGig(payload);
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs', { method: 'POST', body: payload });
      expect(result?.id).toBe('new');
      expect(store.gigs.map((g) => g.id)).toEqual(['new', 'old']);
    });

    it('unwraps a { data } envelope', async () => {
      mockFetch.mockResolvedValue({ data: makeGig({ id: 'new' }) });
      const store = setup();
      const result = await store.createGig(payload);
      expect(result?.id).toBe('new');
      expect(store.gigs[0].id).toBe('new');
    });

    it('returns null and does not mutate the list on error', async () => {
      mockFetch.mockRejectedValue(new Error('boom'));
      const store = setup();
      store.gigs = [makeGig({ id: 'old' })];
      const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
      expect(await store.createGig(payload)).toBeNull();
      expect(store.gigs.map((g) => g.id)).toEqual(['old']);
      spy.mockRestore();
    });
  });

  describe('updateGig', () => {
    it('PUTs and replaces the gig in the list', async () => {
      const updated = makeGig({ id: 'gig-1', event_name: 'Renamed' });
      mockFetch.mockResolvedValue(updated);
      const store = setup();
      store.gigs = [makeGig({ id: 'gig-1' }), makeGig({ id: 'gig-2' })];
      const result = await store.updateGig('gig-1', { event_name: 'Renamed' });
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/gig-1', {
        method: 'PUT',
        body: { event_name: 'Renamed', updated_at: null },
      });
      expect(result?.event_name).toBe('Renamed');
      expect(store.gigs[0].event_name).toBe('Renamed');
      expect(store.gigs[1].id).toBe('gig-2');
    });

    it('upserts the gig into the list when it was not present', async () => {
      mockFetch.mockResolvedValue(makeGig({ id: 'gig-9', event_name: 'X' }));
      const store = setup();
      store.gigs = [makeGig({ id: 'gig-1' })];
      await store.updateGig('gig-9', { event_name: 'X' });
      expect(store.gigs.map((g) => g.id)).toEqual(['gig-1', 'gig-9']);
    });

    it('returns null on error', async () => {
      mockFetch.mockRejectedValue(new Error('boom'));
      const store = setup();
      const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
      expect(await store.updateGig('gig-1', {})).toBeNull();
      spy.mockRestore();
    });
  });

  describe('deleteGig', () => {
    it('DELETEs and removes the gig from the list', async () => {
      mockFetch.mockResolvedValue(undefined);
      const store = setup();
      store.gigs = [makeGig({ id: 'gig-1' }), makeGig({ id: 'gig-2' })];
      expect(await store.deleteGig('gig-1')).toBe(true);
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/gig-1', { method: 'DELETE' });
      expect(store.gigs.map((g) => g.id)).toEqual(['gig-2']);
    });

    it('returns false on error', async () => {
      mockFetch.mockRejectedValue(new Error('boom'));
      const store = setup();
      const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
      expect(await store.deleteGig('gig-1')).toBe(false);
      spy.mockRestore();
    });
  });

  describe('link actions', () => {
    it('linkVenue POSTs venue_id and is_primary', async () => {
      mockFetch.mockResolvedValue(undefined);
      const store = setup();
      expect(await store.linkVenue('gig-1', 'venue-1', true)).toBe(true);
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/gig-1/link-venue', {
        method: 'POST',
        body: { venue_id: 'venue-1', is_primary: true },
      });
    });

    it('linkContact POSTs contact_id and role', async () => {
      mockFetch.mockResolvedValue(undefined);
      const store = setup();
      expect(await store.linkContact('gig-1', 'contact-1', 'promoter')).toBe(true);
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/gig-1/link-contact', {
        method: 'POST',
        body: { contact_id: 'contact-1', role: 'promoter' },
      });
    });

    it('linkTracklist POSTs to the nested route', async () => {
      mockFetch.mockResolvedValue(undefined);
      const store = setup();
      expect(await store.linkTracklist('gig-1', 'tl-1')).toBe(true);
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/gig-1/tracklists/tl-1', {
        method: 'POST',
      });
    });

    it('unlinkTracklist DELETEs the nested route', async () => {
      mockFetch.mockResolvedValue(undefined);
      const store = setup();
      expect(await store.unlinkTracklist('gig-1', 'tl-1')).toBe(true);
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/gig-1/tracklists/tl-1', {
        method: 'DELETE',
      });
    });

    it('each link action returns false on error', async () => {
      mockFetch.mockRejectedValue(new Error('boom'));
      const store = setup();
      const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
      expect(await store.linkVenue('g', 'v', false)).toBe(false);
      expect(await store.linkContact('g', 'c', 'r')).toBe(false);
      expect(await store.linkTracklist('g', 't')).toBe(false);
      expect(await store.unlinkTracklist('g', 't')).toBe(false);
      spy.mockRestore();
    });
  });

  describe('fetchGigsAutocomplete', () => {
    it('returns the array response', async () => {
      mockFetch.mockResolvedValue([makeGig()]);
      const store = setup();
      const result = await store.fetchGigsAutocomplete('ber');
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/gigs/autocomplete?q=ber&limit=5');
      expect(result).toHaveLength(1);
    });

    it('URL-encodes the query', async () => {
      mockFetch.mockResolvedValue([]);
      const store = setup();
      await store.fetchGigsAutocomplete('a b&c');
      expect(mockFetch).toHaveBeenCalledWith(
        `/api/v1/gigs/autocomplete?q=${encodeURIComponent('a b&c')}&limit=5`
      );
    });

    it('returns [] for a non-array response and on error', async () => {
      mockFetch.mockResolvedValue({ data: [] });
      const store = setup();
      expect(await store.fetchGigsAutocomplete('x')).toEqual([]);
      mockFetch.mockRejectedValue(new Error('boom'));
      const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
      expect(await store.fetchGigsAutocomplete('x')).toEqual([]);
      spy.mockRestore();
    });
  });

  describe('generateICalUrl', () => {
    it('embeds the configured ical secret', () => {
      const store = setup('s3cr3t');
      expect(store.generateICalUrl()).toBe('/api/v1/gigs/calendar.ics?secret=s3cr3t');
    });

    it('falls back to an empty secret', () => {
      vi.stubGlobal('useRuntimeConfig', () => ({ public: {} }));
      setActivePinia(createPinia());
      const store = useGigStore();
      expect(store.generateICalUrl()).toBe('/api/v1/gigs/calendar.ics?secret=');
    });
  });

  describe('filters', () => {
    it('setFilter sets a single key', () => {
      const store = setup();
      store.setFilter('from', '2026-01-01');
      expect(store.filters.from).toBe('2026-01-01');
      store.setFilter('fee_max', 500);
      expect(store.filters.fee_max).toBe(500);
    });

    it('clearFilters restores defaults', () => {
      const store = setup();
      store.setFilter('status', 'played');
      store.setFilter('fee_min', 10);
      store.clearFilters();
      expect(store.filters).toEqual({
        status: '',
        venue: '',
        city: '',
        fee_min: null,
        fee_max: null,
        from: '',
        to: '',
      });
    });
  });
});
