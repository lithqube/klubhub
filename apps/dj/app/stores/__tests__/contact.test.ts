import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useContactStore } from '../contact';
import type { Contact } from '../../types/gig';

const mockContact: Contact = {
  id: 'c-1',
  name: 'Promoter Pat',
  company: null,
  email: 'pat@example.com',
  phone: '+49 111',
  type: 'promoter',
  notes: '',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  deleted_at: null,
};

const mockFetch = vi.fn();

function setup() {
  vi.stubGlobal('$fetch', mockFetch);
  setActivePinia(createPinia());
  return useContactStore();
}

describe('useContactStore', () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('starts with an empty contacts list', () => {
    const store = setup();
    expect(store.contacts).toEqual([]);
  });

  it('fetchAutocomplete queries with encoded q and default limit', async () => {
    mockFetch.mockResolvedValue([mockContact]);
    const store = setup();
    const result = await store.fetchAutocomplete('pat');
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/contacts/autocomplete?q=pat&limit=5');
    expect(result).toEqual([mockContact]);
    expect(store.contacts).toEqual([mockContact]);
  });

  it('fetchAutocomplete URL-encodes the query and passes a custom limit', async () => {
    mockFetch.mockResolvedValue([]);
    const store = setup();
    await store.fetchAutocomplete('a b&c', 10);
    expect(mockFetch).toHaveBeenCalledWith(
      `/api/v1/contacts/autocomplete?q=${encodeURIComponent('a b&c')}&limit=10`
    );
  });

  it('fetchAutocomplete unwraps a { data } envelope', async () => {
    mockFetch.mockResolvedValue({ data: [mockContact] });
    const store = setup();
    const result = await store.fetchAutocomplete('pat');
    expect(result).toEqual([mockContact]);
    expect(store.contacts).toEqual([mockContact]);
  });

  it('fetchAutocomplete returns [] and keeps state on error', async () => {
    mockFetch.mockRejectedValue(new Error('boom'));
    const store = setup();
    const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    expect(await store.fetchAutocomplete('pat')).toEqual([]);
    expect(store.contacts).toEqual([]);
    spy.mockRestore();
  });
});
