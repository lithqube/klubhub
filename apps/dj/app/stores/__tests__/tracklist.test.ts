import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useTracklistStore } from '../tracklist';

const mockFetch = vi.fn();

describe('useTracklistStore', () => {
  beforeEach(() => {
    mockFetch.mockReset();
    vi.stubGlobal('$fetch', mockFetch);
    setActivePinia(createPinia());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('initial tracks is empty array', () => {
    const store = useTracklistStore();
    expect(store.tracks).toEqual([]);
  });

  it('initial tracklist is null', () => {
    const store = useTracklistStore();
    expect(store.tracklist).toBeNull();
  });

  it('initial warnings is empty array', () => {
    const store = useTracklistStore();
    expect(store.warnings).toEqual([]);
  });

  it('initial pastTracklists is empty array', () => {
    const store = useTracklistStore();
    expect(store.pastTracklists).toEqual([]);
  });

  it('after setting tracks, reset() returns tracks to []', () => {
    const store = useTracklistStore();
    store.tracks = [{ id: '1' } as any];
    expect(store.tracks).toHaveLength(1);
    store.reset();
    expect(store.tracks).toEqual([]);
  });

  it('reset() also clears tracklist to null', () => {
    const store = useTracklistStore();
    store.tracklist = { id: 'abc' } as any;
    store.reset();
    expect(store.tracklist).toBeNull();
  });

  it('trackCount computed equals tracks.length', () => {
    const store = useTracklistStore();
    expect(store.trackCount).toBe(0);
    store.tracks = [{ id: '1' } as any, { id: '2' } as any];
    expect(store.trackCount).toBe(2);
  });

  describe('fetchTracklist', () => {
    it('returns the tracklist from the { data } envelope', async () => {
      const tracklist = { id: 'tl-1', title: 'Set' };
      mockFetch.mockResolvedValue({ data: tracklist });
      const store = useTracklistStore();
      expect(await store.fetchTracklist('tl-1')).toEqual(tracklist);
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/tracklists/tl-1');
    });

    it('returns null when data is missing', async () => {
      mockFetch.mockResolvedValue({ data: null });
      const store = useTracklistStore();
      expect(await store.fetchTracklist('tl-1')).toBeNull();
    });

    it('returns null on error', async () => {
      mockFetch.mockRejectedValue(new Error('boom'));
      const store = useTracklistStore();
      const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
      expect(await store.fetchTracklist('tl-1')).toBeNull();
      spy.mockRestore();
    });
  });

  describe('loadPastTracklists', () => {
    it('populates pastTracklists from the { data } envelope', async () => {
      const list = [{ id: 'tl-1' }, { id: 'tl-2' }];
      mockFetch.mockResolvedValue({ data: list });
      const store = useTracklistStore();
      await store.loadPastTracklists();
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/tracklists');
      expect(store.pastTracklists).toEqual(list);
    });

    it('tolerates a null data payload', async () => {
      mockFetch.mockResolvedValue({ data: null });
      const store = useTracklistStore();
      await store.loadPastTracklists();
      expect(store.pastTracklists).toEqual([]);
    });

    it('clears pastTracklists on error', async () => {
      mockFetch.mockRejectedValue(new Error('boom'));
      const store = useTracklistStore();
      store.pastTracklists = [{ id: 'tl-1' } as any];
      const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
      await store.loadPastTracklists();
      expect(store.pastTracklists).toEqual([]);
      spy.mockRestore();
    });
  });

  describe('reset', () => {
    it('clears upload state and linked gigs', () => {
      const store = useTracklistStore();
      store.uploadFile = new File(['x'], 'set.txt');
      store.uploadFilename = 'set.txt';
      store.uploadFileSize = '1 B';
      store.warnings = [{ message: 'w' } as any];
      store.linkedGigs = { 'tl-1': ['gig-1'] };
      store.reset();
      expect(store.uploadFile).toBeNull();
      expect(store.uploadFilename).toBe('');
      expect(store.uploadFileSize).toBe('');
      expect(store.warnings).toEqual([]);
    });
  });
});
