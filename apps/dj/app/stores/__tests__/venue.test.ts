import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useVenueStore } from '../venue';
import type { Venue } from '../../types/gig';

const mockVenue: Venue = {
  id: 'v-1',
  name: 'Berghain',
  city: 'Berlin',
  country: 'DE',
  capacity: 1500,
  website: null,
  tech_contact_name: '',
  tech_contact_email: '',
  tech_contact_phone: '',
  notes: '',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  deleted_at: null,
};

const mockFetch = vi.fn();

function setup() {
  vi.stubGlobal('$fetch', mockFetch);
  setActivePinia(createPinia());
  return useVenueStore();
}

describe('useVenueStore', () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('starts with an empty venues list', () => {
    const store = setup();
    expect(store.venues).toEqual([]);
  });

  it('fetchAutocomplete queries with encoded q and default limit', async () => {
    mockFetch.mockResolvedValue([mockVenue]);
    const store = setup();
    const result = await store.fetchAutocomplete('berg');
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/venues/autocomplete?q=berg&limit=5');
    expect(result).toEqual([mockVenue]);
    expect(store.venues).toEqual([mockVenue]);
  });

  it('fetchAutocomplete URL-encodes the query and passes a custom limit', async () => {
    mockFetch.mockResolvedValue([]);
    const store = setup();
    await store.fetchAutocomplete('a b&c', 10);
    expect(mockFetch).toHaveBeenCalledWith(
      `/api/v1/venues/autocomplete?q=${encodeURIComponent('a b&c')}&limit=10`
    );
  });

  it('fetchAutocomplete unwraps a { data } envelope', async () => {
    mockFetch.mockResolvedValue({ data: [mockVenue] });
    const store = setup();
    const result = await store.fetchAutocomplete('berg');
    expect(result).toEqual([mockVenue]);
    expect(store.venues).toEqual([mockVenue]);
  });

  it('fetchAutocomplete returns [] and keeps state on error', async () => {
    mockFetch.mockRejectedValue(new Error('boom'));
    const store = setup();
    const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    expect(await store.fetchAutocomplete('berg')).toEqual([]);
    expect(store.venues).toEqual([]);
    spy.mockRestore();
  });
});
