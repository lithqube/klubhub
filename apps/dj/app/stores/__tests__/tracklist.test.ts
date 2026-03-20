import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useTracklistStore } from '../tracklist';

describe('useTracklistStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
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
});
